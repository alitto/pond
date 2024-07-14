package unboundedchanatomic

import (
	"context"
)

type UnboundedChan[T any] struct {
	out               chan T // channel for read
	bufferHasElements chan *struct{}
	buffer            *LinkedBuffer[T]
}

// NewUnboundedChan creates a new unbounded channel
func NewUnboundedChan[T any](ctx context.Context, initialCapacity int) *UnboundedChan[T] {
	ch := UnboundedChan[T]{
		out:               make(chan T, 10),
		buffer:            NewLinkedBuffer[T](int64(initialCapacity)),
		bufferHasElements: make(chan *struct{}, 1),
	}

	go ch.process(ctx)

	return &ch
}

func (ch *UnboundedChan[T]) Out() <-chan T {
	return ch.out
}

func (ch *UnboundedChan[T]) Write(values ...T) {
	// Append elements to the buffer
	ch.buffer.Write(values...)

	// Notify there are elements in the buffer
	select {
	case ch.bufferHasElements <- &struct{}{}:
	default:
	}
}

func (ch *UnboundedChan[T]) Close() {
	close(ch.bufferHasElements)
}

func (ch *UnboundedChan[T]) process(ctx context.Context) {
	defer close(ch.out)

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch.bufferHasElements:

			if !ok {
				// Channel was closed, exit
				return
			}

			// Attempt to read all pending elements
			for {

				next, err := ch.buffer.Read()
				if err != nil {
					// No more elements to read
					break
				}

				// Attempt to submit the next value
				select {
				case ch.out <- next:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
