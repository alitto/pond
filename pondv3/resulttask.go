package pondv3

import (
	"context"
	"fmt"
)

type ResultTaskContext[R any] interface {
	context.Context
	Err() error
	Result() R
	Wait() (R, error)
}

type resultTaskContext[R any] struct {
	context.Context
	ctxErr error
	err    error
	panic  interface{}
	result R
}

func (c *resultTaskContext[R]) Err() error {
	if c.err != nil {
		return c.err
	}
	if c.panic != nil {
		return fmt.Errorf("%w: %v", ErrPanic, c.panic)
	}
	if c.ctxErr != nil {
		return c.ctxErr
	}
	return nil
}

func (c *resultTaskContext[R]) Result() R {
	return c.result
}

func (c *resultTaskContext[R]) Wait() (R, error) {
	<-c.Done()
	return c.Result(), c.Err()
}

func wrapResultTask[T TaskWithResult[R], R any](task T, ctx context.Context) (func(), ResultTaskContext[R]) {
	ctx, cancel := context.WithCancel(ctx)

	taskCtx := &resultTaskContext[R]{
		Context: ctx,
	}

	wrappedTask := func() {
		defer func() {
			if p := recover(); p != nil {
				taskCtx.panic = p
			}
			taskCtx.ctxErr = taskCtx.Context.Err()
			cancel()
		}()

		switch t := any(task).(type) {
		case func() R:
			taskCtx.result = t()
		case func(context.Context) R:
			taskCtx.result = t(taskCtx)
		case func() (R, error):
			taskCtx.result, taskCtx.err = t()
		case func(context.Context) (R, error):
			taskCtx.result, taskCtx.err = t(taskCtx)
		default:
			panic(fmt.Sprintf("Unsupported task type: %#v", task))
		}
	}

	return wrappedTask, taskCtx
}
