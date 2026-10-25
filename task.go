package pond

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
)

var ErrPanic = errors.New("task panicked")

var ErrContextCanceled = errors.New("context canceled")

type wrappedTask[R any, C func(error) | func(R, error)] struct {
	task          any
	callback      C
	ctx           context.Context
	panicRecovery bool
}

func (t wrappedTask[R, C]) Run() error {
	var result R
	var err error

	if t.ctx != nil {
		if ctxErr := t.ctx.Err(); ctxErr != nil {
			err = errors.Join(ErrContextCanceled, ctxErr)
		}
	}

	if err == nil {
		result, err = invokeTask[R](t.task, t.panicRecovery)
	}

	switch c := any(t.callback).(type) {
	case func(error):
		c(err)
	case func(R, error):
		c(result, err)
	default:
		panic(fmt.Sprintf("unsupported callback type: %#v", t.callback))
	}

	return err
}

func wrapTask[R any, C func(error) | func(R, error)](task any, callback C, ctx context.Context, panicRecovery bool) func() error {
	wrapped := &wrappedTask[R, C]{
		task:          task,
		callback:      callback,
		ctx:           ctx,
		panicRecovery: panicRecovery,
	}

	return wrapped.Run
}

func invokeTask[R any](task any, panicRecovery bool) (output R, err error) {
	if panicRecovery {
		defer func() {
			if p := recover(); p != nil {
				if e, ok := p.(error); ok {
					err = fmt.Errorf("%w: %w, %s", ErrPanic, e, debug.Stack())
				} else {
					err = fmt.Errorf("%w: %v, %s", ErrPanic, p, debug.Stack())
				}
				return
			}
		}()
	}

	switch t := any(task).(type) {
	case func():
		t()
	case func() error:
		err = t()
	case func() R:
		output = t()
	case func() (R, error):
		output, err = t()
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
	return
}
