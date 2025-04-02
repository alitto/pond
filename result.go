package pond

import (
	"context"

	"github.com/alitto/pond/v2/internal/future"
)

// ResultPool is a pool that can be used to submit tasks that return a result.
type ResultPool[R any] interface {
	basePool

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete and get the result.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, this method will return ErrPoolStopped.
	Submit(task func() R) Result[R]

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete and get the result.
	// The task function must return a result and an error.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, this method will return ErrPoolStopped.
	SubmitErr(task func() (R, error)) Result[R]

	// Creates a new subpool with the specified maximum concurrency and options.
	NewSubpool(maxConcurrency int, options ...Option) ResultPool[R]

	// Creates a new task group.
	NewGroup() ResultTaskGroup[R]

	// Creates a new task group with the specified context.
	NewGroupContext(ctx context.Context) ResultTaskGroup[R]
}

type resultPool[R any] struct {
	*pool
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

	p.pool.submit(wrapped)

	return future
}

func (p *resultPool[R]) NewSubpool(maxConcurrency int, options ...Option) ResultPool[R] {
	return newResultPool[R](maxConcurrency, p.pool, options...)
}

func newResultPool[R any](maxConcurrency int, parent *pool, options ...Option) *resultPool[R] {
	return &resultPool[R]{
		pool: newPool(maxConcurrency, parent, options...),
	}
}

// NewResultPool creates a new result pool with the given maximum concurrency and options.
// Result pools are generic pools that can be used to submit tasks that return a result.
// The new maximum concurrency must be greater than or equal to 0 (0 means no limit).
func NewResultPool[R any](maxConcurrency int, options ...Option) ResultPool[R] {
	return newResultPool[R](maxConcurrency, nil, options...)
}
