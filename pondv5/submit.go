package pondv5

import (
	"context"
	"errors"
	"fmt"
)

var ErrPanic = errors.New("task panicked")
var ErrContextCanceled = errors.New("context canceled")

type VoidFunc interface {
	~func() | ~func(context.Context) | ~func() error | ~func(context.Context) error
}

type FuncWithResult[R any] interface {
	~func() R | ~func(context.Context) R | ~func() (R, error) | ~func(context.Context) (R, error)
}

func SubmitVoid[T VoidFunc](pool Pool, task T) VoidTaskContext {
	wrappedTask, callCtx := wrapVoidFunc(task, pool.Context())
	pool.Submit(wrappedTask)
	return callCtx
}

func Submit[R any, T FuncWithResult[R]](pool Pool, task T) TaskContext[R] {
	wrappedTask, taskCtx := wrapFunc[R](task, pool.Context())
	pool.Submit(wrappedTask)
	return taskCtx
}

type ResultOrErr interface {
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

type TaskContext[R any] interface {
	context.Context
	Wait() (R, error)
}

type taskContext[R any] struct {
	context.Context
}

func (c *taskContext[R]) Wait() (R, error) {
	<-c.Context.Done()
	cause := context.Cause(c.Context)
	if r, ok := cause.(*resultOrErr); ok {
		return r.Result().(R), r.Err()
	}
	var zero R
	return zero, cause
}

type VoidTaskContext interface {
	context.Context
	Wait() error
}

type voidTaskContext struct {
	context.Context
}

func (c *voidTaskContext) Wait() error {
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

func wrapFunc[R any, T FuncWithResult[R]](task T, parentCtx context.Context) (func(), *taskContext[R]) {
	childCtx, cancel := context.WithCancelCause(parentCtx)

	callCtx := &taskContext[R]{
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

func wrapVoidFunc[T VoidFunc](task T, parentCtx context.Context) (func(), *voidTaskContext) {
	childCtx, cancel := context.WithCancelCause(parentCtx)

	callCtx := &voidTaskContext{
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
