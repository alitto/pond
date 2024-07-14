package pondv4

import (
	"context"
	"fmt"
)

type Func[I any, O any] interface {
	~func(I) O | ~func(I, context.Context) O | ~func(I) (O, error) | ~func(I, context.Context) (O, error)
}

type FuncPool[I any, O any, F Func[I, O]] interface {
	Invoke(input I) InvocationContext[O]
	StopAndWait()
}

type funcPool[I any, O any, F Func[I, O]] struct {
	ctx        context.Context
	pool       WorkerPool
	workerFunc F
}

func (p *funcPool[I, O, F]) Invoke(input I) InvocationContext[O] {
	wrappedFunc, invocationCtx := wrapFunc[I, O, F](p.workerFunc, input, p.ctx)
	p.pool.Submit(wrappedFunc)
	return invocationCtx
}

func (p *funcPool[I, O, F]) StopAndWait() {
	p.pool.StopAndWait()
}

func NewFuncPool[I any, O any, F Func[I, O]](ctx context.Context, maxWorkers int, workerFunc F) FuncPool[I, O, F] {
	return &funcPool[I, O, F]{
		ctx:        ctx,
		pool:       NewPool(ctx, maxWorkers),
		workerFunc: workerFunc,
	}
}

type InvocationContext[R any] interface {
	context.Context
	Err() error
	Result() R
	Wait() (R, error)
}

type invocationContext[R any] struct {
	context.Context
	ctxErr error
	err    error
	panic  interface{}
	result R
}

func (c *invocationContext[R]) Err() error {
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

func (c *invocationContext[R]) Result() R {
	return c.result
}

func (c *invocationContext[R]) Wait() (R, error) {
	<-c.Done()
	return c.Result(), c.Err()
}

func wrapFunc[I any, O any, F Func[I, O]](workerFunc F, input I, ctx context.Context) (func(), InvocationContext[O]) {
	ctx, cancel := context.WithCancel(ctx)

	invocationCtx := &invocationContext[O]{
		Context: ctx,
	}

	wrappedFunc := func() {
		defer func() {
			if p := recover(); p != nil {
				invocationCtx.panic = p
			}
			invocationCtx.ctxErr = invocationCtx.Context.Err()
			cancel()
		}()

		switch t := any(workerFunc).(type) {
		case func(I) O:
			invocationCtx.result = t(input)
		case func(I, context.Context) O:
			invocationCtx.result = t(input, invocationCtx)
		case func(I) (O, error):
			invocationCtx.result, invocationCtx.err = t(input)
		case func(I, context.Context) (O, error):
			invocationCtx.result, invocationCtx.err = t(input, invocationCtx)
		default:
			panic(fmt.Sprintf("Unsupported worker func type: %#v", workerFunc))
		}
	}

	return wrappedFunc, invocationCtx
}
