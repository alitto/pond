package pond

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var ErrDispatcherClosed = errors.New("dispatcher has been closed")

type dispatcher[T any] struct {
	bufferHasElements chan struct{}
	buffer            *linkedBuffer[T]
	dispatchFunc      func([]T)
	waitGroup         sync.WaitGroup
	batchSize         int
	closed            atomic.Bool
}

// newDispatcher creates a generic dispatcher that can receive values from multiple goroutines in a thread-safe manner
// and process each element serially using the dispatchFunc
func newDispatcher[T any](ctx context.Context, dispatchFunc func([]T), batchSize int) *dispatcher[T] {
	dispatcher := &dispatcher[T]{
		buffer:            newLinkedBuffer[T](batchSize, 10*batchSize),
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
func (d *dispatcher[T]) Write(values ...T) error {
	// Check if the dispatcher has been closed
	if d.closed.Load() {
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
func (d *dispatcher[T]) WriteCount() uint64 {
	return d.buffer.WriteCount()
}

// ReadCount returns the number of elements read from the dispatcher's buffer
func (d *dispatcher[T]) ReadCount() uint64 {
	return d.buffer.ReadCount()
}

// Len returns the number of elements in the dispatcher's buffer
func (d *dispatcher[T]) Len() uint64 {
	return d.buffer.Len()
}

// Close closes the dispatcher
func (d *dispatcher[T]) Close() {
	d.closed.Store(true)
	close(d.bufferHasElements)
}

// CloseAndWait closes the dispatcher and waits for all pending elements to be processed
func (d *dispatcher[T]) CloseAndWait() {
	d.Close()
	d.waitGroup.Wait()
}

// run reads elements from the buffer and processes them using the dispatchFunc
func (d *dispatcher[T]) run(ctx context.Context) {
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

			if !ok {
				// Channel was closed, read all remaining elements and exit
				return
			}
		}
	}
}
