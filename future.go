package pond

import (
	"context"
)

type Async interface {
	// Context returns the context associated with this future.
	Context() context.Context

	// Wait waits for the future to complete and returns any error that occurred.
	Wait() error
}

type Output[O any] interface {
	Async

	// Get waits for the future to complete and returns the output and any error that occurred.
	Get() (O, error)
}
