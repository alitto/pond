package pond

import "context"

type withOutput[O any] struct {
	ctx context.Context
}

func (w withOutput[O]) NewPool(maxConcurrency int) GenericPool[O] {
	return newGenericPool[O](maxConcurrency, w.ctx, nil)
}

func (w withOutput[O]) Go(task func()) {
	defaultPool.Go(task)
}

func (w withOutput[O]) Submit(task func() O) Output[O] {
	return submitTask[func() O, O](task, defaultPool)
}

func (w withOutput[O]) SubmitErr(task func() (O, error)) Output[O] {
	return submitTask[func() (O, error), O](task, defaultPool)
}

func (w withOutput[O]) Group() GenericTaskGroup[O] {
	return NewGenericTaskGroup[O](defaultPool)
}

func (w withOutput[O]) Subpool(maxConcurrency int) GenericPool[O] {
	return newGenericPool[O](maxConcurrency, w.ctx, defaultPool)
}

func (w withOutput[O]) WithContext(ctx context.Context) withOutput[O] {
	return withOutput[O]{ctx: ctx}
}

func WithOutput[O any]() withOutput[O] {
	return withOutput[O]{
		ctx: context.Background(),
	}
}

type withContext struct {
	ctx         context.Context
	defaultPool Pool
}

func (w withContext) NewPool(maxConcurrency int) Pool {
	return newPool(maxConcurrency, w.ctx, nil)
}

func (w withContext) Go(task func()) {
	w.defaultPool.Go(task)
}

func (w withContext) Submit(task func()) Async {
	return submitTask[func(), struct{}](task, w.defaultPool)
}

func (w withContext) SubmitErr(task func() error) Async {
	return submitTask[func() error, struct{}](task, w.defaultPool)
}

func (w withContext) Group() TaskGroup {
	return NewTaskGroup(w.defaultPool)
}

func (w withContext) Subpool(maxConcurrency int) Pool {
	return newPool(maxConcurrency, w.ctx, w.defaultPool)
}

func WithContext(ctx context.Context) withContext {
	return withContext{
		ctx:         ctx,
		defaultPool: NewPool(DEFAULT_POOL_SIZE),
	}
}
