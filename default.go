package pond

import (
	"context"
)

// defaultPool is the default pool used by the package-level functions.
var defaultPool = newPool(0, context.Background())

func Go(task func()) {
	defaultPool.Go(task)
}

func Submit(task func()) TaskFuture {
	return defaultPool.Submit(task)
}

func SubmitErr(task func() error) TaskFuture {
	return defaultPool.SubmitErr(task)
}

func Group(tasks ...func()) TaskGroup {
	return defaultPool.Group(tasks...)
}

func GroupErr(tasks ...func() error) TaskGroup {
	return defaultPool.GroupErr(tasks...)
}

func Subpool(maxConcurrency int) Pool {
	return defaultPool.Subpool(maxConcurrency)
}
