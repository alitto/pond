package pond

import (
	"errors"
	"fmt"
	"sync"
)

var ErrPanic = errors.New("task panicked")

type subpoolTask[R any] struct {
	task          any
	sem           chan struct{}
	waitGroup     *sync.WaitGroup
	updateMetrics func(error)
}

func (t subpoolTask[R]) Run() {
	defer func() {
		// Release semaphore
		<-t.sem
		// Decrement wait group
		t.waitGroup.Done()
	}()

	_, err := invokeTask[R](t.task)

	if t.updateMetrics != nil {
		t.updateMetrics(err)
	}
}

type wrappedTask struct {
	task     any
	callback func(error)
}

func (t wrappedTask) Run() error {
	_, err := invokeTask[any](t.task)

	t.callback(err)

	return err
}

type wrappedResultTask[R any] struct {
	task     any
	callback func(R, error)
}

func (t wrappedResultTask[R]) Run() error {
	output, err := invokeTask[R](t.task)

	t.callback(output, err)

	return err
}

func wrapTask(task any, callback func(error)) func() error {
	wrapped := &wrappedTask{
		task:     task,
		callback: callback,
	}

	return wrapped.Run
}

func wrapResultTask[R any](task any, callback func(R, error)) func() error {
	wrapped := &wrappedResultTask[R]{
		task:     task,
		callback: callback,
	}

	return wrapped.Run
}

func invokeTask[R any](task any) (output R, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%w: %v", ErrPanic, p)
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
