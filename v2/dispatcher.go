package pond

import (
	"context"
	"sync"
)

type dispatcher[T any] struct {
	bufferHasElements chan struct{}
	buffer            *LinkedBuffer[T]
	dispatchFunc      func([]T)
	waitGroup         sync.WaitGroup
	batchSize         int
}

// newDispatcher creates an object that can receive elements from multiple goroutines in a thread-safe manner
// and process each element serially using the dispatchFunc
func newDispatcher[T any](ctx context.Context, dispatchFunc func([]T), batchSize int) *dispatcher[T] {
	dispatcher := &dispatcher[T]{
		buffer:            NewLinkedBuffer[T](batchSize, 10*batchSize),
		bufferHasElements: make(chan struct{}, 1),
		dispatchFunc:      dispatchFunc,
		batchSize:         batchSize,
	}

	dispatcher.waitGroup.Add(1)
	go dispatcher.process(ctx)

	return dispatcher
}

func (d *dispatcher[T]) Write(values ...T) {
	// Append elements to the buffer
	d.buffer.Write(values)

	// Notify there are elements in the buffer
	select {
	case d.bufferHasElements <- struct{}{}:
	default:
	}
}

func (d *dispatcher[T]) WriteCount() int64 {
	return d.buffer.WriteCount()
}

func (d *dispatcher[T]) ReadCount() int64 {
	return d.buffer.ReadCount()
}

func (d *dispatcher[T]) Len() int64 {
	return d.buffer.Len()
}

func (d *dispatcher[T]) Close() {
	close(d.bufferHasElements)
}

func (d *dispatcher[T]) CloseAndWait() {
	d.Close()
	d.waitGroup.Wait()
}

func (d *dispatcher[T]) process(ctx context.Context) {
	defer d.waitGroup.Done()

	batch := make([]T, d.batchSize)
	var batchSize int

	for {
		select {
		case <-ctx.Done():
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
