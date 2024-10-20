package future

import (
	"context"
)

type FutureResolver func(err error)

// A Future represents a value that will be available in the Future.
// It is always associated with a context that can be used to wait for the value to be available.
// When the parent context is canceled, the Future will be canceled as well.
type Future struct {
	ctx context.Context
}

func (f *Future) Done() <-chan struct{} {
	return f.ctx.Done()
}

func (f *Future) Err() error {
	<-f.ctx.Done()

	cause := context.Cause(f.ctx)
	if cause != nil {
		if resolution, ok := cause.(*futureResolution); ok {
			return resolution.err
		}
	}
	return cause
}

// Wait waits for the future to complete and returns any error that occurred.
func (f *Future) Wait() error {
	return f.Err()
}

func NewFuture(ctx context.Context) (*Future, FutureResolver) {
	childCtx, cancel := context.WithCancelCause(ctx)
	future := &Future{
		ctx: childCtx,
	}
	return future, func(err error) {
		cancel(&futureResolution{
			err: err,
		})
	}
}

type futureResolution struct {
	err error
}

func (v *futureResolution) Error() string {
	if v.err != nil {
		return v.err.Error()
	}
	return "future resolved"
}
