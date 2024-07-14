package pond

import (
	"context"
	"errors"
)

var ErrPanic = errors.New("task panicked")

type VoidFunc interface {
	~func() | ~func(context.Context) | ~func() error | ~func(context.Context) error
}

type FuncWithResult[R any] interface {
	~func() R | ~func(context.Context) R | ~func() (R, error) | ~func(context.Context) (R, error)
}

func SubmitVoid[T VoidFunc](pool Pool, task T) VoidContext {
	wrappedTask, callCtx := wrapVoidFunc(task, pool.Context())
	pool.Submit(wrappedTask)
	return callCtx
}

func Submit[R any, T FuncWithResult[R]](pool Pool, task T) ResultContext[R] {
	wrappedTask, taskCtx := wrapFunc[R](task, pool.Context())
	pool.Submit(wrappedTask)
	return taskCtx
}
