package unboundedchanv3

import (
	"context"
	"sync"
	"sync/atomic"
)

type UnboundedChan[T any] struct {
	bufferHasElements chan *struct{}
	buffer            *LinkedBuffer[T]
	receiveFunc       func([]T)
	waitGroup         sync.WaitGroup
	receiveCount      atomic.Int64
	batchSize         int
}

// NewUnboundedChan creates a new unbounded channel
func NewUnboundedChan[T any](ctx context.Context, receiveFunc func([]T), batchSize int) *UnboundedChan[T] {
	ch := UnboundedChan[T]{
		buffer:            NewLinkedBuffer[T](batchSize, 10*batchSize),
		bufferHasElements: make(chan *struct{}, 1),
		receiveFunc:       receiveFunc,
		batchSize:         batchSize,
	}

	ch.waitGroup.Add(1)
	go ch.process(ctx)

	return &ch
}

func (ch *UnboundedChan[T]) Write(values ...T) {
	// Append elements to the buffer
	ch.buffer.Write(values)

	// Notify there are elements in the buffer
	select {
	case ch.bufferHasElements <- &struct{}{}:
	default:
	}
}

func (ch *UnboundedChan[T]) WriteCount() int64 {
	return ch.buffer.WriteCount()
}

func (ch *UnboundedChan[T]) ReadCount() int64 {
	return ch.buffer.ReadCount()
}

func (ch *UnboundedChan[T]) Len() int64 {
	return ch.buffer.Len()
}

func (ch *UnboundedChan[T]) ReceiveCount() int64 {
	return ch.receiveCount.Load()
}

func (ch *UnboundedChan[T]) Close() {
	close(ch.bufferHasElements)
}

func (ch *UnboundedChan[T]) CloseAndWait() {
	ch.Close()
	ch.waitGroup.Wait()
}

func (ch *UnboundedChan[T]) process(ctx context.Context) {
	defer ch.waitGroup.Done()

	batch := make([]T, ch.batchSize)

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch.bufferHasElements:

			// Attempt to read all pending elements
			for {
				batchSize := ch.buffer.Read(batch)

				if batchSize == 0 {
					break
				}

				// Submit the next batch of values
				ch.receiveFunc(batch[0:batchSize])

				// Increment read count
				ch.receiveCount.Add(int64(batchSize))

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
