package pond

import (
	"context"
	"fmt"
)

type ResultAndErr interface {
	Result() interface{}
	Err() error
}

type resultOrErr struct {
	err    error
	result any
}

func (r *resultOrErr) Result() interface{} {
	return r.result
}

func (r *resultOrErr) Err() error {
	return r.err
}

func (r *resultOrErr) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return fmt.Sprintf("result: %#v", r.result)
}

type ResultContext[R any] interface {
	context.Context
	Wait() (R, error)
}

type resultContext[R any] struct {
	context.Context
}

func (c *resultContext[R]) Wait() (R, error) {
	<-c.Context.Done()
	cause := context.Cause(c.Context)
	if r, ok := cause.(*resultOrErr); ok {
		return r.Result().(R), r.Err()
	}
	var zero R
	return zero, cause
}

type VoidContext interface {
	context.Context
	Wait() error
}

type voidContext struct {
	context.Context
}

func (c *voidContext) Wait() error {
	<-c.Context.Done()
	cause := context.Cause(c.Context)
	if r, ok := cause.(*resultOrErr); ok {
		return r.Err()
	}
	return cause
}

func deferFunc(cancel context.CancelCauseFunc) {
	if p := recover(); p != nil {
		cancel(&resultOrErr{err: fmt.Errorf("%w: %v", ErrPanic, p)})
		return
	}
}

func wrapFunc[R any, T FuncWithResult[R]](task T, parentCtx context.Context) (func(), *resultContext[R]) {
	childCtx, cancel := context.WithCancelCause(parentCtx)

	callCtx := &resultContext[R]{
		Context: childCtx,
	}

	switch t := any(task).(type) {
	case func() R:
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			result := t()
			cancel(&resultOrErr{result: result})
		}, callCtx
	case func(context.Context) R:
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			result := t(callCtx)
			cancel(&resultOrErr{result: result})
		}, callCtx
	case func() (R, error):
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			result, err := t()
			cancel(&resultOrErr{result: result, err: err})
		}, callCtx
	case func(context.Context) (R, error):
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			result, err := t(callCtx)
			cancel(&resultOrErr{result: result, err: err})
		}, callCtx
	default:
		panic(fmt.Sprintf("Unsupported task type: %#v", task))
	}
}

func wrapVoidFunc[T VoidFunc](task T, parentCtx context.Context) (func(), *voidContext) {
	childCtx, cancel := context.WithCancelCause(parentCtx)

	callCtx := &voidContext{
		Context: childCtx,
	}

	switch t := any(task).(type) {
	case func():
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			t()
			cancel(&resultOrErr{})
		}, callCtx
	case func(context.Context):
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			t(callCtx)
			cancel(&resultOrErr{})
		}, callCtx
	case func() error:
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			err := t()
			cancel(&resultOrErr{err: err})
		}, callCtx
	case func(context.Context) error:
		return func() {
			defer func() {
				deferFunc(cancel)
			}()

			err := t(callCtx)
			cancel(&resultOrErr{err: err})
		}, callCtx
	default:
		panic(fmt.Sprintf("Unsupported task type: %#v", task))
	}
}
