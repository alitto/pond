package pond

import (
	"context"

	"github.com/alitto/pond/v2/internal/future"
)

// ResultPool is a pool that can be used to submit tasks that return a result.
type ResultPool[R any] interface {
	basePool

	// Enables debug mode for the pool.
	EnableDebug()

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete and get the result.
	Submit(task func() R) Result[R]

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete and get the result.
	SubmitErr(task func() (R, error)) Result[R]

	// Creates a new subpool with the specified maximum concurrency.
	NewSubpool(maxConcurrency int) ResultPool[R]

	// Creates a new task group.
	NewGroup() ResultTaskGroup[R]

	// Creates a new task group with the specified context.
	NewGroupContext(ctx context.Context) ResultTaskGroup[R]
}

type resultPool[R any] struct {
	*pool
}

func (d *resultPool[R]) EnableDebug() {
	d.dispatcher.EnableDebug()
}

func (p *resultPool[R]) NewGroup() ResultTaskGroup[R] {
	return newResultTaskGroup[R](p.pool, p.Context())
}

func (p *resultPool[R]) NewGroupContext(ctx context.Context) ResultTaskGroup[R] {
	return newResultTaskGroup[R](p.pool, ctx)
}

func (p *resultPool[R]) Submit(task func() R) Result[R] {
	return p.submit(task)
}

func (p *resultPool[R]) SubmitErr(task func() (R, error)) Result[R] {
	return p.submit(task)
}

func (p *resultPool[R]) submit(task any) Result[R] {
	future, resolve := future.NewValueFuture[R](p.Context())

	wrapped := wrapTask[R, func(R, error)](task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *resultPool[R]) NewSubpool(maxConcurrency int) ResultPool[R] {
	return newResultSubpool[R](maxConcurrency, p.Context(), p.pool)
}

func NewResultPool[R any](maxConcurrency int, options ...Option) ResultPool[R] {
	return &resultPool[R]{
		pool: newPool(maxConcurrency, options...),
	}
}
