package pond

import (
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/future"
)

var ErrPanic = errors.New("task panicked")

type subpoolTask[O any] struct {
	task          any
	sem           chan struct{}
	waitGroup     *sync.WaitGroup
	updateMetrics func(error)
}

func (t subpoolTask[O]) Run() {
	defer func() {
		// Release semaphore
		<-t.sem
		// Decrement wait group
		t.waitGroup.Done()
	}()

	_, err := invokeTask[O](t.task)

	if t.updateMetrics != nil {
		t.updateMetrics(err)
	}
}

type wrappedTask[O any] struct {
	task     any
	callback func(O, error)
}

func (t wrappedTask[O]) Run() {
	output, err := invokeTask[O](t.task)

	t.callback(output, err)
}

func (t wrappedTask[O]) RunErr() error {
	output, err := invokeTask[O](t.task)

	t.callback(output, err)

	return err
}

func wrapTask[O any](task any, callback func(O, error)) func() {
	wrapped := &wrappedTask[O]{
		task:     task,
		callback: callback,
	}

	return wrapped.Run
}

func wrapTaskErr[O any](task any, callback func(O, error)) func() error {
	wrapped := &wrappedTask[O]{
		task:     task,
		callback: callback,
	}

	return wrapped.RunErr
}

func invokeTask[O any](task any) (output O, err error) {
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
	case func() O:
		output = t()
	case func() (O, error):
		output, err = t()
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
	return
}

func submitTask[T TaskFunc[O], O any](task T, pool AbstractPool) Output[O] {
	future, resolve := future.NewFuture[O](pool.Context())

	wrapped := wrapTask(task, resolve)

	pool.Go(wrapped)

	return future
}
