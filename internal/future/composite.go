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
}

type compositeErrorResolution struct {
	index int
	err   error
}

func (e *compositeErrorResolution) Error() string {
	return e.err.Error()
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

func (f *CompositeFuture[V]) Done(count int) <-chan struct{} {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	ch := make(chan struct{})

	err := context.Cause(f.ctx)

	// Return immediately if the context is already canceled or the count is already reached
	if len(f.resolutions) >= count || err != nil {
		close(ch)
		return ch
	}

	// Register a listener
	f.listeners = append(f.listeners, waitListener{
		count: count,
		ch:    ch,
	})

	return ch
}

func (f *CompositeFuture[V]) Context() context.Context {
	return f.ctx
}

func (f *CompositeFuture[V]) Cancel(cause error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Cancel the context
	f.cancel(cause)

	// Notify listeners
	f.notifyListeners()
}

func (f *CompositeFuture[V]) Wait(count int) ([]V, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	result, err := f.getResult(count)
	if result != nil || err != nil {
		return result, err
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

	return f.getResult(count)
}

func (f *CompositeFuture[V]) resolve(index int, value V, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if index < 0 {
		panic(fmt.Errorf("index must be greater than or equal to 0"))
	}

	// Cancel the context if an error occurred
	if err != nil {
		f.cancel(&compositeErrorResolution{
			index: index,
			err:   err,
		})
	} else if context.Cause(f.ctx) == nil {
		// Save the resolution
		f.resolutions = append(f.resolutions, compositeResolution[V]{
			index: index,
			value: value,
		})
	}

	// Notify listeners
	f.notifyListeners()
}

func (f *CompositeFuture[V]) getResult(count int) (values []V, err error) {

	cause := context.Cause(f.ctx)

	// If we have enough results, return them
	if cause != nil || len(f.resolutions) >= count {
		// Get sorted resolution values
		values = make([]V, count)
		for _, resolution := range f.resolutions {
			if resolution.index < count {
				values[resolution.index] = resolution.value
			}
		}
	}

	if cause == nil {
		return
	}

	if errorResolution, ok := cause.(*compositeErrorResolution); ok {
		// Unwrap the error resolution
		err = errorResolution.err
	} else if len(f.resolutions) < count {
		// If the context is canceled and we have collected enough results, return nil error
		// because we assume that context cancellation happened after the last resolution.
		err = cause
	}

	return
}

func (f *CompositeFuture[V]) notifyListeners() {

	err := context.Cause(f.ctx)

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
