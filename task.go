package pond

import (
	"errors"
	"fmt"
	"runtime/debug"
)

var ErrPanic = errors.New("task panicked")

type wrappedTask[R any, C func(error) | func(R, error)] struct {
	task     any
	callback C
}

func (t wrappedTask[R, C]) Run() error {
	result, err := invokeTask[R](t.task)

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

func wrapTask[R any, C func(error) | func(R, error)](task any, callback C) func() error {
	wrapped := &wrappedTask[R, C]{
		task:     task,
		callback: callback,
	}

	return wrapped.Run
}

func invokeTask[R any](task any) (output R, err error) {
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
