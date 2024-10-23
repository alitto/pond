package pond

import (
	"errors"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/future"
)

// TaskGroup represents a group of tasks that can be executed concurrently.
// The group can be waited on to block until all tasks have completed.
// If any of the tasks return an error, the group will return the first error encountered.
type TaskGroup interface {

	// Submits a task to the group.
	Submit(tasks ...func()) TaskGroup

	// Submits a task to the group that can return an error.
	SubmitErr(tasks ...func() error) TaskGroup

	// Waits for all tasks in the group to complete.
	Wait() error
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

	// Waits for all tasks in the group to complete.
	Wait() ([]O, error)
}

type result[O any] struct {
	Output O
	Err    error
}

type abstractTaskGroup[T func() | func() O, E func() error | func() (O, error), O any] struct {
	pool           *pool
	nextIndex      atomic.Int64
	future         *future.CompositeFuture[*result[O]]
	futureResolver future.CompositeFutureResolver[*result[O]]
}

func (g *abstractTaskGroup[T, E, O]) Submit(tasks ...T) *abstractTaskGroup[T, E, O] {
	if len(tasks) == 0 {
		panic(errors.New("no tasks provided"))
	}

	for _, task := range tasks {
		g.submit(task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) SubmitErr(tasks ...E) *abstractTaskGroup[T, E, O] {
	if len(tasks) == 0 {
		panic(errors.New("no tasks provided"))
	}

	for _, task := range tasks {
		g.submit(task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) submit(task any) {
	index := int(g.nextIndex.Add(1) - 1)

	err := g.pool.Go(func() {
		// Check if the context has been cancelled to prevent running tasks that are not needed
		if err := g.future.Context().Err(); err != nil {
			g.futureResolver(index, &result[O]{
				Err: err,
			}, err)
			return
		}

		// Invoke the task
		output, err := invokeTask[O](task)

		g.futureResolver(index, &result[O]{
			Output: output,
			Err:    err,
		}, err)
	})

	if err != nil {
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

	if err != nil {
		return []O{}, err
	}

	values := make([]O, len(results))

	for i, result := range results {
		values[i] = result.Output
	}

	return values, err
}

func newTaskGroup(pool *pool) TaskGroup {
	future, futureResolver := future.NewCompositeFuture[*result[struct{}]](pool.Context())

	return &taskGroup{
		abstractTaskGroup: abstractTaskGroup[func(), func() error, struct{}]{
			pool:           pool,
			future:         future,
			futureResolver: futureResolver,
		},
	}
}

func newResultTaskGroup[O any](pool *pool) ResultTaskGroup[O] {
	future, futureResolver := future.NewCompositeFuture[*result[O]](pool.Context())

	return &resultTaskGroup[O]{
		abstractTaskGroup: abstractTaskGroup[func() O, func() (O, error), O]{
			pool:           pool,
			future:         future,
			futureResolver: futureResolver,
		},
	}
}
