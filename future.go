package pond

type TaskFuture interface {
	// Wait waits for the task associated to this future to complete and returns any error that occurred.
	Wait() error
}

type ResultFuture[O any] interface {
	// Wait waits for the task associated to this future to complete and returns its result and any error that occurred.
	Wait() (O, error)
}
