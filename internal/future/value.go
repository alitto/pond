package future

import (
	"context"
	"fmt"
)

type ValueFutureResolver[V any] func(value V, err error)

// A Future represents a value that will be available in the Future.
// It is always associated with a context that can be used to wait for the value to be available.
// When the parent context is canceled, the Future will be canceled as well.
type ValueFuture[V any] struct {
	ctx context.Context
}

// Get waits for the future to complete and returns the output and any error that occurred.
func (f *ValueFuture[V]) Wait() (V, error) {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if resolution, ok := cause.(*valueFutureResolution[V]); ok {
		return resolution.value, resolution.err
	}
	var zero V
	return zero, cause
}

func NewValueFuture[V any](ctx context.Context) (*ValueFuture[V], ValueFutureResolver[V]) {
	childCtx, cancel := context.WithCancelCause(ctx)
	future := &ValueFuture[V]{
		ctx: childCtx,
	}
	return future, func(value V, err error) {
		cancel(&valueFutureResolution[V]{
			value: value,
			err:   err,
		})
	}
}

type valueFutureResolution[V any] struct {
	value V
	err   error
}

func (v *valueFutureResolution[V]) Error() string {
	if v.err != nil {
		return v.err.Error()
	}
	return fmt.Sprintf("future value: %v", v.value)
}
