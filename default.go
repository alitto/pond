package pond

// Default pool size
const DEFAULT_POOL_SIZE = 10000

var defaultPool = NewPool(DEFAULT_POOL_SIZE)

func Go(task func()) {
	defaultPool.Go(task)
}

func Submit(task func()) Async {
	return defaultPool.Submit(task)
}

func SubmitErr(task func() error) Async {
	return defaultPool.SubmitErr(task)
}

func Group() TaskGroup {
	return NewTaskGroup(defaultPool)
}

func Subpool(maxConcurrency int) Pool {
	return defaultPool.Subpool(maxConcurrency)
}
