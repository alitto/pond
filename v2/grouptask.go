package pond

import (
	"context"
	"sync"
)

type GroupTask[I, O any] interface {
	Pool() Pool
	Context() context.Context
	Inputs() []I
	WithPool(pool Pool) GroupTask[I, O]
	WithContext(ctx context.Context) GroupTask[I, O]
	WithInputs(input ...I) GroupTask[I, O]
	WithMaxConcurrency(maxConcurrency int) GroupTask[I, O]
	Run() TaskContext[[]O]
}

type groupTask[I, O any] struct {
	task[I, O]
	inputs []I
}

func (t *groupTask[I, O]) clone() *groupTask[I, O] {
	return &groupTask[I, O]{
		task:   t.task,
		inputs: t.inputs,
	}
}

func (t *groupTask[I, O]) Inputs() []I {
	return t.inputs
}

func (t *groupTask[I, O]) WithContext(ctx context.Context) GroupTask[I, O] {
	if ctx == nil {
		panic("context cannot be nil")
	}
	task := t.clone()
	task.ctx = ctx
	return task
}

func (t *groupTask[I, O]) WithPool(pool Pool) GroupTask[I, O] {
	if pool == nil {
		panic("pool cannot be nil")
	}
	task := t.clone()
	task.pool = pool
	return task
}

func (t *groupTask[I, O]) WithMaxConcurrency(maxConcurrency int) GroupTask[I, O] {
	if maxConcurrency < 0 {
		panic("maxConcurrency must be greater than 0")
	}
	task := t.clone()
	task.pool = t.Pool().Subpool(maxConcurrency)
	return task
}

func (t *groupTask[I, O]) WithInputs(inputs ...I) GroupTask[I, O] {
	if inputs == nil {
		panic("inputs cannot be nil")
	}
	return &groupTask[I, O]{
		task:   t.task,
		inputs: inputs,
	}
}

func (t *groupTask[I, O]) Run() TaskContext[[]O] {

	childCtx, cancel := context.WithCancelCause(t.Context())

	pending := len(t.inputs)
	mutex := sync.Mutex{}
	outputOrErr := &outputOrErr[[]O]{
		output: make([]O, pending),
	}

	taskGroupCtx := &taskContext[[]O]{
		Context: childCtx,
	}

	for i, in := range t.inputs {
		index := i

		t.Pool().Submit(func() {
			output, err := t.normalFunc(in, childCtx)

			mutex.Lock()
			if childCtx.Err() != nil {
				// Context has already been canceled
				mutex.Unlock()
				return
			}
			if err != nil {
				// Save the first error returned by any task
				outputOrErr.err = err
			}

			// Save task output
			outputOrErr.output[index] = output

			// Decrement pending count
			pending--

			if pending == 0 || outputOrErr.err != nil {
				// Once all tasks have completed or one of the returned an error, cancel the task group context
				cancel(outputOrErr)
			}
			mutex.Unlock()
		})
	}

	return taskGroupCtx
}
