package pond

// Task represents a task that can be waited on. If the task fails, the error can be retrieved.
type Task interface {

	// Done returns a channel that is closed when the task is complete or has failed.
	Done() <-chan struct{}

	// Wait waits for the task to complete and returns any error that occurred.
	Wait() error
}

// ResultTask represents a task that yields a result. If the task fails, the error can be retrieved.
type ResultTask[R any] interface {

	// Done returns a channel that is closed when the task is complete or has failed.
	Done() <-chan struct{}

	// Wait waits for the task to complete and returns the result and any error that occurred.
	Wait() (R, error)
}

// Result is deprecated. Use ResultTask instead.
// This interface is maintained for backward compatibility.
//
// Deprecated: Use ResultTask instead.
type Result[R any] interface {

	// Done returns a channel that is closed when the task is complete or has failed.
	Done() <-chan struct{}

	// Wait waits for the task to complete and returns the result and any error that occurred.
	Wait() (R, error)
}
