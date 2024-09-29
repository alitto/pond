package future

import (
	"context"
	"sync/atomic"
)

type CompositeFutureResolver[V any] func(index int, value V, err error)

type CompositeFuture[V any] struct {
	*Future[[]V]
	resolver FutureResolver[[]V]
	values   []V
	pending  atomic.Int64
}

func NewCompositeFuture[V any](ctx context.Context, count int) (*CompositeFuture[V], CompositeFutureResolver[V]) {
	childFuture, resolver := NewFuture[[]V](ctx)

	future := &CompositeFuture[V]{
		Future:   childFuture,
		resolver: resolver,
		values:   make([]V, count),
	}

	future.pending.Store(int64(count))

	return future, future.resolve
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {

	if index < 0 {
		panic("index must be greater than or equal to 0")
	}
	if index >= len(f.values) {
		panic("index must be less than the number of values")
	}

	pending := f.pending.Add(-1)

	// Save the value
	f.values[index] = value

	if pending == 0 || err != nil {
		if err != nil {
			f.resolver([]V{}, err)
		} else {
			f.resolver(f.values, nil)
		}
	}
}
