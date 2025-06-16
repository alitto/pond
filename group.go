package pond

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/future"
)

var ErrGroupStopped = errors.New("task group stopped")

// TaskGroup represents a group of tasks that can be executed concurrently.
// The group can be waited on to block until all tasks have completed.
// If any of the tasks return an error, the group will return the first error encountered.
type TaskGroup interface {

	// Submits a task to the group.
	Submit(tasks ...func()) TaskGroup

	// Submits a task to the group that can return an error.
	SubmitErr(tasks ...func() error) TaskGroup

	// Waits for all tasks in the group to complete.
	// If any of the tasks return an error, the group will return the first error encountered.
	// If the context is cancelled, the group will return the context error.
	// If the group is stopped, the group will return ErrGroupStopped.
	// If a task is running when the context is cancelled or the group is stopped, the task will be allowed to complete before returning.
	Wait() error

	// Returns a channel that is closed when all tasks in the group have completed, a task returns an error, or the group is stopped.
	Done() <-chan struct{}

	// Stops the group and cancels all remaining tasks. Running tasks are not interrupted.
	Stop()

	// Returns the context associated with this group.
	// This context will be cancelled when either the parent context is cancelled
	// or any task in the group returns an error, whichever comes first.
	Context() context.Context
}

// ResultTaskGroup represents a group of tasks that can be executed concurrently.
// As opposed to TaskGroup, the tasks in a ResultTaskGroup yield a result.
// The group can be waited on to block until all tasks have completed.
// If any of the tasks return an error, the group will return the first error encountered.
type ResultTaskGroup[O any] interface {

	// Submits a task to the group.
	Submit(tasks ...func() O) ResultTaskGroup[O]

	// Submits a task to the group that can return an error.
	SubmitErr(tasks ...func() (O, error)) ResultTaskGroup[O]

	// Waits for all tasks in the group to complete and returns the results of each task in the order they were submitted.
	// If any of the tasks return an error, the group will return the first error encountered.
	// If the context is cancelled, the group will return the context error.
	// If the group is stopped, the group will return ErrGroupStopped.
	// If a task is running when the context is cancelled or the group is stopped, the task will be allowed to complete before returning.
	Wait() ([]O, error)

	// Returns a channel that is closed when all tasks in the group have completed, a task returns an error, or the group is stopped.
	Done() <-chan struct{}

	// Stops the group and cancels all remaining tasks. Running tasks are not interrupted.
	Stop()
}

type result[O any] struct {
	Output O
	Err    error
}

type abstractTaskGroup[T func() | func() O, E func() error | func() (O, error), O any] struct {
	pool           *pool
	nextIndex      atomic.Int64
	taskWaitGroup  sync.WaitGroup
	future         *future.CompositeFuture[*result[O]]
	futureResolver future.CompositeFutureResolver[*result[O]]
}

func (g *abstractTaskGroup[T, E, O]) Done() <-chan struct{} {
	return g.future.Done(int(g.nextIndex.Load()))
}

func (g *abstractTaskGroup[T, E, O]) Stop() {
	g.future.Cancel(ErrGroupStopped)
}

func (g *abstractTaskGroup[T, E, O]) Context() context.Context {
	return g.future.Context()
}

func (g *abstractTaskGroup[T, E, O]) Submit(tasks ...T) *abstractTaskGroup[T, E, O] {
	for _, task := range tasks {
		g.submit(task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) SubmitErr(tasks ...E) *abstractTaskGroup[T, E, O] {
	for _, task := range tasks {
		g.submit(task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) submit(task any) {
	index := int(g.nextIndex.Add(1) - 1)

	g.taskWaitGroup.Add(1)

	err := g.pool.submit(func() error {
		defer g.taskWaitGroup.Done()

		// Check if the context has been cancelled to prevent running tasks that are not needed
		if err := g.future.Context().Err(); err != nil {
			g.futureResolver(index, &result[O]{
				Err: err,
			}, err)
			return err
		}

		// Invoke the task
		output, err := invokeTask[O](task)

		g.futureResolver(index, &result[O]{
			Output: output,
			Err:    err,
		}, err)

		return err
	}, g.pool.nonBlocking)

	if err != nil {
		g.taskWaitGroup.Done()

		g.futureResolver(index, &result[O]{
			Err: err,
		}, err)
	}
}

type taskGroup struct {
	abstractTaskGroup[func(), func() error, struct{}]
}

func (g *taskGroup) Submit(tasks ...func()) TaskGroup {
	g.abstractTaskGroup.Submit(tasks...)
	return g
}

func (g *taskGroup) SubmitErr(tasks ...func() error) TaskGroup {
	g.abstractTaskGroup.SubmitErr(tasks...)
	return g
}

func (g *taskGroup) Wait() error {
	_, err := g.future.Wait(int(g.nextIndex.Load()))
	// This wait group could reach zero before the future is resolved if called in between tasks being submitted and the future being resolved.
	// That's why we wait for the future to be resolved before waiting for the wait group.
	g.taskWaitGroup.Wait()
	return err
}

type resultTaskGroup[O any] struct {
	abstractTaskGroup[func() O, func() (O, error), O]
}

func (g *resultTaskGroup[O]) Submit(tasks ...func() O) ResultTaskGroup[O] {
	g.abstractTaskGroup.Submit(tasks...)
	return g
}

func (g *resultTaskGroup[O]) SubmitErr(tasks ...func() (O, error)) ResultTaskGroup[O] {
	g.abstractTaskGroup.SubmitErr(tasks...)
	return g
}

func (g *resultTaskGroup[O]) Wait() ([]O, error) {
	results, err := g.future.Wait(int(g.nextIndex.Load()))

	// This wait group could reach zero before the future is resolved if called in between tasks being submitted and the future being resolved.
	// That's why we wait for the future to be resolved before waiting for the wait group.
	g.taskWaitGroup.Wait()

	values := make([]O, len(results))

	for i, result := range results {
		if result != nil {
			values[i] = result.Output
		}
	}

	return values, err
}

func newTaskGroup(pool *pool, ctx context.Context) TaskGroup {
	future, futureResolver := future.NewCompositeFuture[*result[struct{}]](ctx)

	return &taskGroup{
		abstractTaskGroup: abstractTaskGroup[func(), func() error, struct{}]{
			pool:           pool,
			future:         future,
			futureResolver: futureResolver,
		},
	}
}

func newResultTaskGroup[O any](pool *pool, ctx context.Context) ResultTaskGroup[O] {
	future, futureResolver := future.NewCompositeFuture[*result[O]](ctx)

	return &resultTaskGroup[O]{
		abstractTaskGroup: abstractTaskGroup[func() O, func() (O, error), O]{
			pool:           pool,
			future:         future,
			futureResolver: futureResolver,
		},
	}
}
