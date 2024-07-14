package pondv3

func Submit[T TaskWithoutResult](pool WorkerPool, task T) TaskContext {
	wrappedTask, taskCtx := wrapTask[T](task, pool.Context())
	pool.Submit(wrappedTask)
	return taskCtx
}

func SubmitAndGet[R any, T TaskWithResult[R]](pool WorkerPool, task T) ResultTaskContext[R] {
	wrappedTask, taskCtx := wrapResultTask[T, R](task, pool.Context())
	pool.Submit(wrappedTask)
	return taskCtx
}
