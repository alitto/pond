package future

import (
	"context"
	"fmt"
	"sync"
)

type CompositeFutureResolver[V any] func(index int, value V, err error)

type CompositeFuture[V any] struct {
	*ValueFuture[[]V]
	resolver ValueFutureResolver[[]V]
	values   []V
	pending  int
	mutex    sync.Mutex
}

func NewCompositeFuture[V any](ctx context.Context, count int) (*CompositeFuture[V], CompositeFutureResolver[V]) {
	childFuture, resolver := NewValueFuture[[]V](ctx)

	future := &CompositeFuture[V]{
		ValueFuture: childFuture,
		resolver:    resolver,
		values:      make([]V, count),
		pending:     count,
	}

	return future, future.resolve
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {

	if index < 0 {
		panic(fmt.Errorf("index must be greater than or equal to 0"))
	}
	if index >= len(f.values) {
		panic(fmt.Errorf("index must be less than %d", len(f.values)))
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.pending--

	// Save the value
	f.values[index] = value

	if f.pending == 0 || err != nil {
		if err != nil {
			f.resolver([]V{}, err)
		} else {
			f.resolver(f.values, nil)
		}
	}
}
