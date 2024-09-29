package pond

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/future"
)

var ErrPanic = errors.New("task panicked")

type subpoolTask[O any] struct {
	task      any
	sem       chan struct{}
	waitGroup *sync.WaitGroup
}

func (t subpoolTask[O]) Run(ctx context.Context) {
	defer func() {
		// Release semaphore
		<-t.sem
		// Decrement wait group
		t.waitGroup.Done()
	}()

	invokeTask[O](t.task, ctx)
}

type wrappedTask[O any] struct {
	task      any
	callbacks []func(O, error)
}

func (t wrappedTask[O]) Run(ctx context.Context) {
	output, err := invokeTask[O](t.task, ctx)

	if t.callbacks == nil {
		return
	}

	for _, callback := range t.callbacks {
		callback(output, err)
	}
}

func wrapTask[O any](task any, callbacks ...func(O, error)) *wrappedTask[O] {
	return &wrappedTask[O]{
		task:      task,
		callbacks: callbacks,
	}
}

func validateTask[O any](task any) {

	switch any(task).(type) {
	case func():
		return
	case func(context.Context):
		return
	case func() error:
		return
	case func(context.Context) error:
		return
	case func() O:
		return
	case func(context.Context) O:
		return
	case func() (O, error):
		return
	case func(context.Context) (O, error):
		return
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
}

func invokeTask[O any](task any, ctx context.Context) (output O, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%w: %v", ErrPanic, p)
			return
		}
	}()

	switch t := any(task).(type) {
	case func():
		t()
	case func(context.Context):
		t(ctx)
	case func() error:
		err = t()
	case func(context.Context) error:
		err = t(ctx)
	case func() O:
		output = t()
	case func(context.Context) O:
		output = t(ctx)
	case func() (O, error):
		output, err = t()
	case func(context.Context) (O, error):
		output, err = t(ctx)
	default:
		panic(fmt.Sprintf("unsupported task type: %#v", task))
	}
	return
}

func submitTyped[O any](task any, ctx context.Context, runner TaskRunner) Future[O] {

	future, resolve := future.NewFuture[O](ctx)

	futureTask := wrapTask(task, resolve)

	runner.Go(futureTask.Run)

	return future
}
