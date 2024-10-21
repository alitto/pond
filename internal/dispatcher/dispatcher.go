package dispatcher

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/linkedbuffer"
)

var ErrDispatcherClosed = errors.New("dispatcher has been closed")

type Dispatcher[T any] struct {
	ctx               context.Context
	bufferHasElements chan struct{}
	buffer            *linkedbuffer.LinkedBuffer[T]
	dispatchFunc      func([]T)
	waitGroup         sync.WaitGroup
	batchSize         int
	closed            atomic.Bool
}

// NewDispatcher creates a generic dispatcher that can receive values from multiple goroutines in a thread-safe manner
// and process each element serially using the dispatchFunc
func NewDispatcher[T any](ctx context.Context, dispatchFunc func([]T), batchSize int) *Dispatcher[T] {
	dispatcher := &Dispatcher[T]{
		ctx:               ctx,
		buffer:            linkedbuffer.NewLinkedBuffer[T](10, batchSize),
		bufferHasElements: make(chan struct{}, 1),
		dispatchFunc:      dispatchFunc,
		batchSize:         batchSize,
		closed:            atomic.Bool{},
	}

	dispatcher.waitGroup.Add(1)
	go dispatcher.run(ctx)

	return dispatcher
}

// Write writes values to the dispatcher
func (d *Dispatcher[T]) Write(values ...T) error {
	// Check if the dispatcher has been closed
	if d.closed.Load() || d.ctx.Err() != nil {
		return ErrDispatcherClosed
	}

	// Append elements to the buffer
	d.buffer.Write(values)

	// Notify there are elements in the buffer
	select {
	case d.bufferHasElements <- struct{}{}:
	default:
	}

	return nil
}

// WriteCount returns the number of elements written to the dispatcher
func (d *Dispatcher[T]) WriteCount() uint64 {
	return d.buffer.WriteCount()
}

// ReadCount returns the number of elements read from the dispatcher's buffer
func (d *Dispatcher[T]) ReadCount() uint64 {
	return d.buffer.ReadCount()
}

// Len returns the number of elements in the dispatcher's buffer
func (d *Dispatcher[T]) Len() uint64 {
	return d.buffer.Len()
}

// Close closes the dispatcher
func (d *Dispatcher[T]) Close() {
	d.closed.Store(true)
	close(d.bufferHasElements)
}

// CloseAndWait closes the dispatcher and waits for all pending elements to be processed
func (d *Dispatcher[T]) CloseAndWait() {
	d.Close()
	d.waitGroup.Wait()
}

// run reads elements from the buffer and processes them using the dispatchFunc
func (d *Dispatcher[T]) run(ctx context.Context) {
	defer d.waitGroup.Done()

	batch := make([]T, d.batchSize)
	var batchSize int

	for {

		// Prioritize context cancellation over dispatching
		select {
		case <-ctx.Done():
			// Context cancelled, exit
			return
		default:
		}

		select {
		case <-ctx.Done():
			// Context cancelled, exit
			return
		case _, ok := <-d.bufferHasElements:

			// Attempt to read all pending elements
			for {
				batchSize = d.buffer.Read(batch)

				if batchSize == 0 {
					break
				}

				// Submit the next batch of values
				d.dispatchFunc(batch[0:batchSize])

				// Reset batch
				batch = batch[:0]
			}

			if !ok || d.closed.Load() {
				// Channel was closed, read all remaining elements and exit
				return
			}
		}
	}
}
