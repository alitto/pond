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

type waitListener struct {
	count int
	ch    chan struct{}
}

type CompositeFuture[V any] struct {
	ctx         context.Context
	cancel      context.CancelCauseFunc
	resolutions []compositeResolution[V]
	mutex       sync.Mutex
	listeners   []waitListener
}

func NewCompositeFuture[V any](ctx context.Context) (*CompositeFuture[V], CompositeFutureResolver[V]) {
	childCtx, cancel := context.WithCancelCause(ctx)

	future := &CompositeFuture[V]{
		ctx:         childCtx,
		cancel:      cancel,
		resolutions: make([]compositeResolution[V], 0),
	}

	return future, future.resolve
}

func (f *CompositeFuture[V]) Wait(count int) ([]V, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	err := context.Cause(f.ctx)

	if len(f.resolutions) >= count || err != nil {
		if err != nil {
			return []V{}, err
		}

		// Get sorted results
		result := make([]V, count)
		for _, resolution := range f.resolutions {
			if resolution.index < count {
				result[resolution.index] = resolution.value
			}
		}

		return result, nil
	}

	// Register a listener
	ch := make(chan struct{})
	f.listeners = append(f.listeners, waitListener{
		count: count,
		ch:    ch,
	})

	f.mutex.Unlock()

	// Wait for the listener to be notified or the context to be canceled
	select {
	case <-ch:
	case <-f.ctx.Done():
	}

	f.mutex.Lock()

	if err := context.Cause(f.ctx); err != nil {
		return []V{}, err
	}

	// Get sorted results
	result := make([]V, count)
	for _, resolution := range f.resolutions {
		if resolution.index < count {
			result[resolution.index] = resolution.value
		}
	}

	return result, nil
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if index < 0 {
		panic(fmt.Errorf("index must be greater than or equal to 0"))
	}

	// Save the resolution
	f.resolutions = append(f.resolutions, compositeResolution[V]{
		index: index,
		value: value,
		err:   err,
	})

	// Cancel the context if an error occurred
	if err != nil {
		f.cancel(err)
	}

	// Notify listeners
	for i := 0; i < len(f.listeners); i++ {
		listener := f.listeners[i]

		if err != nil || listener.count <= len(f.resolutions) {
			close(listener.ch)
			f.listeners = append(f.listeners[:i], f.listeners[i+1:]...)
			i--
		}
	}
}
