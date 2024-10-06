package pond

import (
	"sync"

	"github.com/alitto/pond/v2/internal/future"
)

/**
 * TaskGroup is a collection of tasks that can be submitted to a pool.
 * The tasks are executed concurrently and the results are collected in the order they are received.
 */
type AbstractTaskGroup[O any] interface {
	Get() ([]O, error)
	Wait() error
	All() ([]*Result[O], error)
	Go()
}

type TaskGroup interface {
	AbstractTaskGroup[struct{}]
	Submit(tasks ...func())
	SubmitErr(tasks ...func() error)
}

type GenericTaskGroup[O any] interface {
	AbstractTaskGroup[O]
	Submit(tasks ...func() O)
	SubmitErr(tasks ...func() (O, error))
}

type Result[O any] struct {
	Output O
	Err    error
}

type taskGroup[T TaskFunc[O], E TaskFunc[O], O any] struct {
	pool  AbstractPool
	mutex sync.Mutex
	tasks []any
}

func (g *taskGroup[T, E, O]) Submit(tasks ...T) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range tasks {
		g.tasks = append(g.tasks, task)
	}
}

func (g *taskGroup[T, E, O]) SubmitErr(tasks ...E) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range tasks {
		g.tasks = append(g.tasks, task)
	}
}

func (g *taskGroup[T, E, O]) Get() ([]O, error) {
	return g.submit().Get()
}

func (g *taskGroup[T, E, O]) Wait() error {
	return g.submit().Wait()
}

func (g *taskGroup[T, E, O]) All() ([]*Result[O], error) {
	results, err := g.submitAll().Get()

	if err == nil {
		for _, result := range results {
			if result.Err != nil {
				err = result.Err
				break
			}
		}
	}

	return results, err
}

func (g *taskGroup[T, E, O]) submit() Output[[]O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[O](g.pool.Context(), len(g.tasks))

	for i, task := range g.tasks {
		index := i
		g.pool.Go(func() {
			output, err := invokeTask[O](task)

			resolve(index, output, err)
		})
	}

	// Reset tasks
	g.tasks = g.tasks[:0]

	return future
}

func (g *taskGroup[T, E, O]) submitAll() Output[[]*Result[O]] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[*Result[O]](g.pool.Context(), len(g.tasks))

	for i, task := range g.tasks {
		index := i
		g.pool.Go(func() {
			output, err := invokeTask[O](task)

			resolve(index, &Result[O]{
				Output: output,
				Err:    err,
			}, nil)
		})
	}

	// Reset tasks
	g.tasks = g.tasks[:0]

	return future
}

func (g *taskGroup[T, E, O]) Go() {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range g.tasks {
		g.pool.Go(func() {
			invokeTask[O](task)
		})
	}

	// Reset tasks
	g.tasks = g.tasks[:0]
}

func NewTaskGroup(pool AbstractPool) TaskGroup {
	return &taskGroup[func(), func() error, struct{}]{
		pool: pool,
	}
}

func NewGenericTaskGroup[O any](pool AbstractPool) GenericTaskGroup[O] {
	return &taskGroup[func() O, func() (O, error), O]{
		pool: pool,
	}
}
