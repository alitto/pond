package pond

import "context"

// Background pool size
const BACKGROUND_POOL_SIZE = 10000

var backgroundPool = newPool[any](context.Background(), BACKGROUND_POOL_SIZE, nil)

type TaskFunc[O any] interface {
	func() | func(context.Context) | func() error | func(context.Context) error | func() O | func(context.Context) O | func() (O, error) | func(context.Context) (O, error)
}

func Background() Pool[any] {
	return backgroundPool
}

func Submit[O any, T TaskFunc[O]](task T) Future[O] {

	future, resolve := newFuture[O](backgroundPool.Context())

	futureTask := futureTask[O]{
		task:    task,
		ctx:     future.Context(),
		resolve: resolve,
	}

	backgroundPool.Go(futureTask.Run)

	return future
}

func Go(task any) {
	backgroundPool.Go(task)
}
