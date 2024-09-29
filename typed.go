package pond

func Typed[O any](pool Pool[any]) Pool[O] {
	return &typedPool[O]{
		Pool: pool,
	}
}

type typedPool[O any] struct {
	Pool[any]
}

func (p *typedPool[O]) Group() TaskGroup[O] {
	return newTaskGroup[O](p.Context(), p.Pool)
}

func (p *typedPool[O]) Submit(task any) Future[O] {
	return submitTyped[O](task, p.Context(), p.Pool)
}

func (p *typedPool[O]) Subpool(maxConcurrency int, options ...PoolOption) Pool[O] {

	opts := &PoolOptions{
		Context: p.Context(),
	}

	for _, option := range options {
		option(opts)
	}

	return newPool[O](maxConcurrency, opts, p.Pool)
}
