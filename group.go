package pond

import (
	"sync"

	"github.com/alitto/pond/v2/internal/future"
)

/**
 * TaskGroup is a collection of tasks that can be submitted to a pool.
 * The tasks are executed concurrently and the results are collected in the order they are received.
 */
type TaskGroup interface {
	Submit(tasks ...func()) TaskGroup
	SubmitErr(tasks ...func() error) TaskGroup
	Wait() error
	WaitAll() ([]error, error)
}

type GenericTaskGroup[O any] interface {
	Submit(tasks ...func() O) GenericTaskGroup[O]
	SubmitErr(tasks ...func() (O, error)) GenericTaskGroup[O]
	Wait() ([]O, error)
	WaitAll() ([]*Result[O], error)
}

type Result[O any] struct {
	Output O
	Err    error
}

type abstractTaskGroup[T TaskFunc[O], E TaskFunc[O], O any] struct {
	pool  abstractPool
	mutex sync.Mutex
	tasks []any
}

func (g *abstractTaskGroup[T, E, O]) Submit(tasks ...T) *abstractTaskGroup[T, E, O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range tasks {
		g.tasks = append(g.tasks, task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) SubmitErr(tasks ...E) *abstractTaskGroup[T, E, O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range tasks {
		g.tasks = append(g.tasks, task)
	}

	return g
}

func (g *abstractTaskGroup[T, E, O]) submit() ResultFuture[[]O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[O](g.pool.Context(), len(g.tasks))

	for i, task := range g.tasks {
		index := i
		err := g.pool.Go(func() {
			output, err := invokeTask[O](task)

			resolve(index, output, err)
		})

		if err != nil {
			var zero O
			resolve(index, zero, err)
		}
	}

	// Reset tasks
	g.tasks = g.tasks[:0]

	return future
}

func (g *abstractTaskGroup[T, E, O]) submitAll() ResultFuture[[]*Result[O]] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[*Result[O]](g.pool.Context(), len(g.tasks))

	for i, task := range g.tasks {
		index := i
		err := g.pool.Go(func() {
			output, err := invokeTask[O](task)

			resolve(index, &Result[O]{
				Output: output,
				Err:    err,
			}, nil)
		})

		if err != nil {
			resolve(index, &Result[O]{
				Err: err,
			}, nil)
		}
	}

	// Reset tasks
	g.tasks = g.tasks[:0]

	return future
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
	_, err := g.abstractTaskGroup.submit().Wait()
	return err
}

func (g *taskGroup) WaitAll() ([]error, error) {
	results, err := g.submitAll().Wait()

	errors := make([]error, len(results))

	for i, result := range results {
		// Get the first error
		if err == nil && result.Err != nil {
			err = result.Err
		}

		errors[i] = result.Err
	}

	return errors, err
}

type genericTaskGroup[O any] struct {
	abstractTaskGroup[func() O, func() (O, error), O]
}

func (g *genericTaskGroup[O]) Submit(tasks ...func() O) GenericTaskGroup[O] {
	g.abstractTaskGroup.Submit(tasks...)
	return g
}

func (g *genericTaskGroup[O]) SubmitErr(tasks ...func() (O, error)) GenericTaskGroup[O] {
	g.abstractTaskGroup.SubmitErr(tasks...)
	return g
}

func (g *genericTaskGroup[O]) Wait() ([]O, error) {
	return g.abstractTaskGroup.submit().Wait()
}

func (g *genericTaskGroup[O]) WaitAll() ([]*Result[O], error) {
	results, err := g.submitAll().Wait()

	if err == nil {
		// Get the first error
		for _, result := range results {
			if result.Err != nil {
				err = result.Err
				break
			}
		}
	}

	return results, err
}

func newTaskGroup(pool abstractPool) TaskGroup {
	return &taskGroup{
		abstractTaskGroup: abstractTaskGroup[func(), func() error, struct{}]{
			pool: pool,
		},
	}
}

func newGenericTaskGroup[O any](pool abstractPool) GenericTaskGroup[O] {
	return &genericTaskGroup[O]{
		abstractTaskGroup: abstractTaskGroup[func() O, func() (O, error), O]{
			pool: pool,
		},
	}
}
