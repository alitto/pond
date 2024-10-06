package pond

import (
	"context"

	"github.com/alitto/pond/v2/internal/future"
)

type TaskFunc[O any] interface {
	func() | func() error | func() O | func() (O, error)
}

type GenericPool[O any] interface {
	AbstractPool

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	Submit(task func() O) Output[O]

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	SubmitErr(task func() (O, error)) Output[O]

	// Creates a new subpool with the specified maximum concurrency.
	Subpool(maxConcurrency int) GenericPool[O]

	// Creates a new task group.
	Group() GenericTaskGroup[O]
}

type genericPool[O any] struct {
	*pool
}

func (p *genericPool[O]) Group() GenericTaskGroup[O] {
	return NewGenericTaskGroup[O](p)
}

func (p *genericPool[O]) Submit(task func() O) Output[O] {
	future, resolve := future.NewFuture[O](p.Context())

	wrapped := wrapTask(task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *genericPool[O]) SubmitErr(task func() (O, error)) Output[O] {
	future, resolve := future.NewFuture[O](p.Context())

	wrapped := wrapTaskErr(task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *genericPool[O]) Subpool(maxConcurrency int) GenericPool[O] {
	return newGenericPool[O](maxConcurrency, p.Context(), p)
}

// Typed creates a typed pool from an existing pool.
func newGenericPool[O any](maxConcurrency int, ctx context.Context, parent AbstractPool) GenericPool[O] {
	return &genericPool[O]{
		pool: newPool(maxConcurrency, ctx, parent),
	}
}
