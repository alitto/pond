package pondv3

import (
	"context"
	"errors"
	"fmt"
)

var ErrPanic = errors.New("task panicked")
var ErrContextCanceled = errors.New("context canceled")

type TaskWithoutResult interface {
	~func() | ~func(context.Context) | ~func() error | ~func(context.Context) error
}

type TaskWithResult[R any] interface {
	~func() R | ~func(context.Context) R | ~func() (R, error) | ~func(context.Context) (R, error)
}

type TaskContext interface {
	context.Context
	Err() error
	Wait() error
}

type taskContext struct {
	context.Context
	ctxErr error
	err    error
	panic  interface{}
}

func (c *taskContext) Err() error {
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

func (c *taskContext) Wait() error {
	<-c.Done()
	return c.Err()
}

func wrapTask[T TaskWithoutResult](task T, ctx context.Context) (func(), TaskContext) {
	ctx, cancel := context.WithCancel(ctx)

	taskCtx := &taskContext{
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
		case func():
			t()
		case func(context.Context):
			t(taskCtx)
		case func() error:
			taskCtx.err = t()
		case func(context.Context) error:
			taskCtx.err = t(taskCtx)
		default:
			panic(fmt.Sprintf("Unsupported task type: %#v", task))
		}
	}

	return wrappedTask, taskCtx
}

type TaskFunc[T TaskWithoutResult] func(context.Context) error

func Task[T TaskWithoutResult](task T) TaskFunc[T] {
	return func(ctx context.Context) (err error) {
		switch t := any(task).(type) {
		case func():
			t()
		case func(context.Context):
			t(ctx)
		case func() error:
			err = t()
		case func(context.Context) error:
			err = t(ctx)
		default:
			panic(fmt.Sprintf("Unsupported task type: %#v", task))
		}
		return
	}
}

type ResultTaskFunc[T TaskWithResult[R], R any] func(context.Context) (R, error)

func ResultTask[R any, T TaskWithResult[R]](task T) ResultTaskFunc[T, R] {
	return func(ctx context.Context) (result R, err error) {
		switch t := any(task).(type) {
		case func() R:
			result = t()
		case func(context.Context) R:
			result = t(ctx)
		case func() (R, error):
			result, err = t()
		case func(context.Context) (R, error):
			result, err = t(ctx)
		default:
			panic(fmt.Sprintf("Unsupported task type: %#v", task))
		}
		return
	}
}
