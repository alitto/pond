package pool

// defaultPool is the default pool used by the package-level functions.
var defaultPool = newPool(0, nil)

// Submit submits a task to the default pool and returns a future that can be used to wait for the task to complete.
func Submit(task func()) Task {
	return defaultPool.Submit(task)
}

// SubmitErr submits a task to the default pool and returns a future that can be used to wait for the task to complete.
func SubmitErr(task func() error) Task {
	return defaultPool.SubmitErr(task)
}

// NewGroup creates a new task group with the default pool.
func NewGroup() TaskGroup {
	return defaultPool.NewGroup()
}

// NewSubpool creates a new subpool with the default pool.
func NewSubpool(maxConcurrency int) Pool {
	return defaultPool.NewSubpool(maxConcurrency)
}
