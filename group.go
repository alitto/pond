package pond

import (
	"context"
	"sync"

	"github.com/alitto/pond/v2/internal/future"
)

/**
 * TaskGroup is a collection of tasks that can be submitted to a pool.
 * The tasks are executed concurrently and the results are collected in the order they are received.
 */
type TaskGroup[O any] interface {
	Add(tasks ...any)
	Get() ([]O, error)
	Wait() error
	GetAll() ([]*Result[O], error)
	WaitAll() error
	Go()
}

type taskGroup[O any] struct {
	ctx    context.Context
	runner TaskRunner
	mutex  sync.Mutex
	tasks  []any
}

func (g *taskGroup[O]) Add(tasks ...any) {
	for _, task := range tasks {
		validateTask[O](task)
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.tasks = append(g.tasks, tasks...)
}

func (g *taskGroup[O]) Get() ([]O, error) {
	return g.submit().Get()
}

func (g *taskGroup[O]) Wait() error {
	return g.submit().Wait()
}

func (g *taskGroup[O]) GetAll() ([]*Result[O], error) {
	return g.submitAll().Get()
}

func (g *taskGroup[O]) WaitAll() error {
	return g.submitAll().Wait()
}

func (g *taskGroup[O]) submit() Future[[]O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[O](g.ctx, len(g.tasks))

	for i, task := range g.tasks {
		index := i
		g.runner.Go(func() {
			output, err := invokeTask[O](task, future.Context())

			resolve(index, output, err)
		})
	}

	// Reset tasks
	g.tasks = g.tasks[:0]

	return future
}

func (g *taskGroup[O]) submitAll() Future[[]*Result[O]] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := future.NewCompositeFuture[*Result[O]](g.ctx, len(g.tasks))

	for i, task := range g.tasks {
		index := i
		g.runner.Go(func() {
			output, err := invokeTask[O](task, future.Context())

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

func (g *taskGroup[O]) Go() {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, task := range g.tasks {
		g.runner.Go(task)
	}

	// Reset tasks
	g.tasks = g.tasks[:0]
}

func newTaskGroup[O any](ctx context.Context, pool TaskRunner) *taskGroup[O] {
	return &taskGroup[O]{
		ctx:    ctx,
		runner: pool,
	}
}
