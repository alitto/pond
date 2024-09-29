package pond

import (
	"context"
)

// Background pool size
const BACKGROUND_POOL_SIZE = 10000

var backgroundPool = NewPool(BACKGROUND_POOL_SIZE)

type TaskFunc[O any] interface {
	func() | func(context.Context) | func() error | func(context.Context) error | func() O | func(context.Context) O | func() (O, error) | func(context.Context) (O, error)
}

func Submit(task any) Future[any] {
	return backgroundPool.Submit(task)
}

func Go(task any) {
	backgroundPool.Go(task)
}

func Group() TaskGroup[any] {
	return backgroundPool.Group()
}

func TypedSubmit[O any, T TaskFunc[O]](task T) Future[O] {
	return submitTyped[O](task, backgroundPool.Context(), backgroundPool)
}

func TypedGroup[O any]() TaskGroup[O] {
	return newTaskGroup[O](backgroundPool.Context(), backgroundPool)
}
