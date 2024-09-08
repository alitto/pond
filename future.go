package pond

import (
	"context"
	"fmt"
)

/**
 * Interface representing a future value.
 *
 * @param O The type of the output of the future
 */
type Future[O any] interface {
	// Returns the context associated with this future.
	Context() context.Context

	// Waits for the future to complete and returns any error that occurred.
	Wait() error

	// Waits for the future to complete and returns the output and any error that occurred.
	Get() (O, error)
}

// future is an implementation of the Future interface.
// It is used to represent a value that will be available in the future.
// It has a context that can be used to wait for the value to be available.
type future[O any] struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
}

type futureResult[O any] struct {
	output O
	err    error
}

// Error returns a string representation of the future result.
func (r *futureResult[O]) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return fmt.Sprintf("result: %#v", r.output)
}

// Context returns the context associated with this future.
func (f *future[O]) Context() context.Context {
	return f.ctx
}

// Wait waits for the future to complete and returns any error that occurred.
func (f *future[O]) Wait() error {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if r, ok := cause.(*futureResult[O]); ok {
		return r.err
	}
	return cause
}

// Get waits for the future to complete and returns the output and any error that occurred.
func (f *future[O]) Get() (O, error) {
	<-f.ctx.Done()
	cause := context.Cause(f.ctx)
	if r, ok := cause.(*futureResult[O]); ok {
		return r.output, r.err
	}
	var zero O
	return zero, cause
}

func (f *future[O]) resolve(output O, err error) {
	f.cancel(&futureResult[O]{
		output: output,
		err:    err,
	})
}

func newFuture[O any](ctx context.Context) (Future[O], func(output O, err error)) {
	childCtx, cancel := context.WithCancelCause(ctx)
	future := &future[O]{
		ctx:    childCtx,
		cancel: cancel,
	}
	return future, future.resolve
}

// mappedFuture is a future that maps the output of another future using a function.
type mappedFuture[O, I any] struct {
	Future[I]
	mapFunc func(I) O
}

// Get waits for the future to complete and returns the mapped output and any error that occurred.
func (f *mappedFuture[O, I]) Get() (O, error) {
	output, err := f.Future.Get()
	var mappedOutput O
	if err == nil {
		mappedOutput = f.mapFunc(output)
	}
	return mappedOutput, err
}

/**
 * Maps the output of a future to a new value using the provided function.
 * The function will only be called if the future completes successfully.
 * Example:
 * ```
 * future := pond.Submit(func() int {
 *     return 5
 * })
 * mappedFuture := pond.Map(future, func(i int) string {
 *     return fmt.Sprintf("The value is %d", i)
 * })
 * ```
 *
 * @param f The future to map
 * @param mapFunc The function to map the output of the future
 * @return A new future that will contain the mapped output
 */
func Map[I any, O any](future Future[I], mapFunc func(I) O) Future[O] {

	mappedFuture := &mappedFuture[O, I]{
		Future:  future,
		mapFunc: mapFunc,
	}

	return mappedFuture
}
