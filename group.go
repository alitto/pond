package pond

import (
	"sync"
)

/**
 * TaskGroup is not thread-safe
 */
type TaskGroup[O any] interface {
	Add(tasks ...any)
	Submit() Future[[]O]
	Go()
}

type taskGroup[O any] struct {
	pool  *pool[O]
	mutex sync.Mutex
	tasks []any
}

func (g *taskGroup[O]) Add(tasks ...any) {
	for _, task := range tasks {
		validateTask[O](task)
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.tasks = append(g.tasks, tasks...)
}

func (g *taskGroup[O]) Submit() Future[[]O] {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	future, resolve := newFuture[[]O](g.pool.Context())

	pending := len(g.tasks)
	outputs := make([]O, pending)
	mutex := sync.Mutex{}

	for i, task := range g.tasks {
		index := i

		g.pool.Go(func() {
			output, err := invokeTask[O](task, future.Context())

			mutex.Lock()
			if future.Context().Err() != nil {
				// Future has already resolved
				mutex.Unlock()
				return
			}

			// Save task output
			outputs[index] = output

			// Decrement pending count
			pending--

			if pending == 0 || err != nil {
				// Once all tasks have completed or one of the returned an error, cancel the task group context
				resolve(outputs, err)
			}
			mutex.Unlock()
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
		g.pool.Go(task)
	}

	// Reset tasks
	g.tasks = g.tasks[:0]
}
