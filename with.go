package pond

import (
	"context"
)

type withResultPool[R any] interface {
	GenericPool[R]

	// NewPool creates a new pool with the specified maximum concurrency.
	NewPool(maxConcurrency int) GenericPool[R]

	// WithContext creates a new pool with the specified context.
	WithContext(ctx context.Context) withResultPool[R]
}

type withResult[R any] struct {
	ctx context.Context
	GenericPool[R]
}

func (w withResult[R]) NewPool(maxConcurrency int) GenericPool[R] {
	return newGenericPool[R](maxConcurrency, w.ctx)
}

func (w withResult[R]) WithContext(ctx context.Context) withResultPool[R] {
	return &withResult[R]{
		ctx:         ctx,
		GenericPool: newGenericSubpool[R](0, ctx, defaultPool),
	}
}

func WithResult[R any]() withResultPool[R] {
	return &withResult[R]{
		ctx:         context.Background(),
		GenericPool: newGenericSubpool[R](0, context.Background(), defaultPool),
	}
}

type withContextPool interface {
	Pool

	// NewPool creates a new pool with the specified maximum concurrency.
	NewPool(maxConcurrency int) Pool
}

type withContext struct {
	ctx context.Context
	Pool
}

func (w withContext) NewPool(maxConcurrency int) Pool {
	return newPool(maxConcurrency, w.ctx)
}

func WithContext(ctx context.Context) withContextPool {
	return &withContext{
		ctx:  ctx,
		Pool: newSubpool(0, ctx, defaultPool),
	}
}
