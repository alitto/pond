package future

import (
	"context"
	"fmt"
)

type FutureResolver[V any] func(value V, err error)

// A Future represents a value that will be available in the Future.
// It is always associated with a context that can be used to wait for the value to be available.
// When the parent context is canceled, the Future will be canceled as well.
type Future[V any] struct {
	ctx context.Context
}

// Context returns the context associated with this future.
func (f *Future[V]) Context() context.Context {
	return f.ctx
}

// Wait waits for the future to complete and returns any error that occurred.
func (f *Future[V]) Wait() error {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if resolution, ok := cause.(*valueResolution[V]); ok {
		return resolution.err
	}
	return cause
}

// Get waits for the future to complete and returns the output and any error that occurred.
func (f *Future[V]) Get() (V, error) {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if resolution, ok := cause.(*valueResolution[V]); ok {
		return resolution.value, resolution.err
	}
	var zero V
	return zero, cause
}

func NewFuture[V any](ctx context.Context) (*Future[V], FutureResolver[V]) {
	childCtx, cancel := context.WithCancelCause(ctx)
	future := &Future[V]{
		ctx: childCtx,
	}
	return future, func(value V, err error) {
		cancel(&valueResolution[V]{
			value: value,
			err:   err,
		})
	}
}

type valueResolution[V any] struct {
	value V
	err   error
}

func (v *valueResolution[V]) Error() string {
	if v.err != nil {
		return v.err.Error()
	}
	return fmt.Sprintf("future value: %v", v.value)
}
