package pond

import (
	"context"

	"github.com/alitto/pond/v2/internal/future"
)

type TaskFunc[O any] interface {
	func() | func() error | func() O | func() (O, error)
}

type GenericPool[O any] interface {
	abstractPool

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	Submit(task func() O) ResultFuture[O]

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	SubmitErr(task func() (O, error)) ResultFuture[O]

	// Creates a new subpool with the specified maximum concurrency.
	Subpool(maxConcurrency int) GenericPool[O]

	// Creates a new task group.
	Group(tasks ...func() O) GenericTaskGroup[O]

	// Creates a new task group.
	GroupErr(tasks ...func() (O, error)) GenericTaskGroup[O]
}

type genericPool[O any] struct {
	*pool
}

func (p *genericPool[O]) Group(tasks ...func() O) GenericTaskGroup[O] {
	return newGenericTaskGroup[O](p).Submit(tasks...)
}

func (p *genericPool[O]) GroupErr(tasks ...func() (O, error)) GenericTaskGroup[O] {
	return newGenericTaskGroup[O](p).SubmitErr(tasks...)
}

func (p *genericPool[O]) Submit(task func() O) ResultFuture[O] {
	return p.submit(task)
}

func (p *genericPool[O]) SubmitErr(task func() (O, error)) ResultFuture[O] {
	return p.submit(task)
}

func (p *genericPool[O]) submit(task any) ResultFuture[O] {
	future, resolve := future.NewValueFuture[O](p.Context())

	wrapped := wrapResultTask(task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *genericPool[O]) Subpool(maxConcurrency int) GenericPool[O] {
	return newGenericSubpool[O](maxConcurrency, p.Context(), p.pool)
}

// Typed creates a typed pool from an existing pool.
func newGenericPool[O any](maxConcurrency int, ctx context.Context) GenericPool[O] {
	return &genericPool[O]{
		pool: newPool(maxConcurrency, ctx),
	}
}
