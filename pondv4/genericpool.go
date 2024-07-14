package pondv4

import (
	"context"
	"fmt"
)

type GenericPool interface {
	SubmitAsync(task func())
	Submit(task interface{}) context.Context
	StopAndWait()
}

type genericPool struct {
	pool WorkerPool
}

func (p *genericPool) Context() context.Context {
	return p.pool.Context()
}

func (p *genericPool) SubmitAsync(task func()) {
	p.pool.Submit(task)
}

func (p *genericPool) Submit(task interface{}) context.Context {
	wrappedTask, callCtx := wrapTaskCall[interface{}](task, p.pool.Context())
	p.pool.Submit(wrappedTask)
	return callCtx
}

func (p *genericPool) StopAndWait() {
	p.pool.StopAndWait()
}

func NewGenericPool(ctx context.Context, maxWorkers int) GenericPool {
	return &genericPool{
		pool: NewPool(ctx, maxWorkers),
	}
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

func wrapTaskCall[R any](task interface{}, parentCtx context.Context) (func(), context.Context) {
	callCtx, cancel := context.WithCancelCause(parentCtx)

	wrappedTask := func() {
		var result interface{}
		var err error

		defer func() {
			if p := recover(); p != nil {
				cancel(&resultOrErr{err: fmt.Errorf("%w: %v", ErrPanic, p)})
				return
			}
			if parentCtx.Err() != nil {
				cancel(&resultOrErr{err: parentCtx.Err()})
				return
			}
			cancel(&resultOrErr{err: err, result: result})
		}()

		switch t := any(task).(type) {
		case func():
			t()
		case func(context.Context):
			t(callCtx)
		case func() error:
			err = t()
		case func(context.Context) error:
			err = t(callCtx)
		case func() R:
			result = t()
		case func(context.Context) R:
			result = t(callCtx)
		case func() (R, error):
			result, err = t()
		case func(context.Context) (R, error):
			result, err = t(callCtx)
		default:
			panic(fmt.Sprintf("Unsupported task type: %#v", task))
		}
	}

	return wrappedTask, callCtx
}

func WaitFor[T any](ctx context.Context) (T, error) {
	<-ctx.Done()
	cause := context.Cause(ctx)
	if r, ok := cause.(*resultOrErr); ok {
		return r.Result().(T), r.Err()
	}
	var zero T
	return zero, cause
}

func Wait(ctx context.Context) error {
	<-ctx.Done()
	cause := context.Cause(ctx)
	if r, ok := cause.(*resultOrErr); ok {
		return r.Err()
	}
	return cause
}
