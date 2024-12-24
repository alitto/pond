package pond

import (
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/semaphore"
)

var ErrPanic = errors.New("task panicked")

type subpoolTask[R any] struct {
	task          any
	queueSem      *semaphore.Weighted
	sem           *semaphore.Weighted
	waitGroup     *sync.WaitGroup
	updateMetrics func(error)
}

func (t subpoolTask[R]) Run() {
	defer t.Close()

	// Release task queue semaphore when task is pulled from queue
	if t.queueSem != nil {
		t.queueSem.Release(1)
	}

	_, err := invokeTask[R](t.task)

	if t.updateMetrics != nil {
		t.updateMetrics(err)
	}
}

func (t subpoolTask[R]) Close() {
	// Release semaphore
	t.sem.Release(1)

	// Decrement wait group
	t.waitGroup.Done()
}

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
