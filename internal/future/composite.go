package future

import (
	"context"
	"fmt"
	"sync"
)

type CompositeFutureResolver[V any] func(index int, value V, err error)

type compositeResolution[V any] struct {
	index int
	value V
	err   error
}

type CompositeFuture[V any] struct {
	*ValueFuture[[]V]
	resolver    ValueFutureResolver[[]V]
	resolutions []compositeResolution[V]
	count       int
	mutex       sync.Mutex
}

func NewCompositeFuture[V any](ctx context.Context, count int) (*CompositeFuture[V], CompositeFutureResolver[V]) {
	childFuture, resolver := NewValueFuture[[]V](ctx)

	future := &CompositeFuture[V]{
		ValueFuture: childFuture,
		resolver:    resolver,
		resolutions: make([]compositeResolution[V], 0),
	}

	if count > 0 {
		future.Add(count)
	}

	return future, future.resolve
}

func (f *CompositeFuture[V]) Add(delta int) {
	if delta <= 0 {
		panic(fmt.Errorf("delta must be greater than 0"))
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.count += delta
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if index < 0 {
		panic(fmt.Errorf("index must be greater than or equal to 0"))
	}
	if index >= f.count {
		panic(fmt.Errorf("index must be less than %d", f.count))
	}

	// Save the resolution
	f.resolutions = append(f.resolutions, compositeResolution[V]{
		index: index,
		value: value,
		err:   err,
	})

	pending := f.count - len(f.resolutions)

	if pending == 0 || err != nil {
		if err != nil {
			f.resolver([]V{}, err)
		} else {

			// Sort the resolutions
			values := make([]V, f.count)
			for _, resolution := range f.resolutions {
				values[resolution.index] = resolution.value
			}

			f.resolver(values, nil)
		}
	}
}
