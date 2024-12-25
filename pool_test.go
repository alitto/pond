package pond

import (
	"context"
	"errors"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestPoolSubmit(t *testing.T) {

	pool := NewPool(100)

	var taskCount int = 1000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	pool.Stop().Wait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
}

func TestPoolSubmitAndWait(t *testing.T) {

	pool := NewPool(100)

	done := make(chan int, 1)
	task := pool.Submit(func() {
		done <- 10
	})

	err := task.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 10, <-done)
}

func TestPoolSubmitWithPanic(t *testing.T) {

	pool := NewPool(100)

	sampleErr := errors.New("sample error")

	task := pool.Submit(func() {
		panic(sampleErr)
	})

	err := task.Wait()

	// The returned error should be a wrapped error containing the panic error.
	assert.True(t, errors.Is(err, ErrPanic))
	assert.True(t, errors.Is(err, sampleErr))

	wrappedErrors := (err).(interface {
		Unwrap() []error
	}).Unwrap()

	assert.Equal(t, 2, len(wrappedErrors))
	assert.Equal(t, ErrPanic, wrappedErrors[0])
	assert.Equal(t, sampleErr, wrappedErrors[1])

	matches, err := regexp.MatchString(`task panicked: sample error, goroutine \d+ \[running\]:\n\s*runtime/debug\.Stack\(\)`, err.Error())
	assert.True(t, matches)
	assert.Equal(t, nil, err)
}

func TestPoolSubmitWithErr(t *testing.T) {

	pool := NewPool(100)

	task := pool.SubmitErr(func() error {
		return errors.New("sample error")
	})

	err := task.Wait()

	assert.Equal(t, "sample error", err.Error())
}

func TestPoolWithContextCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(10, WithContext(ctx))

	assert.Equal(t, int64(0), pool.RunningWorkers())
	assert.Equal(t, uint64(0), pool.SubmittedTasks())

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	assert.Equal(t, int64(10), pool.RunningWorkers())
	assert.Equal(t, uint64(taskCount), pool.SubmittedTasks())

	// Cancel the context after 5ms
	time.Sleep(5 * time.Millisecond)
	cancel()

	pool.Stop().Wait()

	assert.True(t, executedCount.Load() < int64(taskCount))
}

func TestPoolMetrics(t *testing.T) {

	pool := NewPool(100)

	// Assert counters
	assert.Equal(t, int64(0), pool.RunningWorkers())
	assert.Equal(t, uint64(0), pool.SubmittedTasks())
	assert.Equal(t, uint64(0), pool.CompletedTasks())
	assert.Equal(t, uint64(0), pool.FailedTasks())
	assert.Equal(t, uint64(0), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.WaitingTasks())

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		n := i
		pool.SubmitErr(func() error {
			executedCount.Add(1)
			if n%2 == 0 {
				return nil
			}
			return errors.New("sample error")
		})
	}

	pool.Stop().Wait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
	assert.Equal(t, int64(0), pool.RunningWorkers())
	assert.Equal(t, uint64(taskCount), pool.SubmittedTasks())
	assert.Equal(t, uint64(taskCount), pool.CompletedTasks())
	assert.Equal(t, uint64(taskCount/2), pool.FailedTasks())
	assert.Equal(t, uint64(taskCount/2), pool.SuccessfulTasks())
}

func TestPoolSubmitOnStoppedPool(t *testing.T) {

	pool := newPool(100)

	pool.Submit(func() {})

	pool.StopAndWait()

	err := pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = pool.Go(func() {})

	assert.Equal(t, ErrPoolStopped, err)
	assert.Equal(t, true, pool.Stopped())
}

func TestNewPoolWithInvalidMaxConcurrency(t *testing.T) {
	assert.PanicsWithError(t, "maxConcurrency must be greater than 0", func() {
		NewPool(-1)
	})
}

func TestPoolStoppedAfterCancel(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(10, WithContext(ctx))

	err := pool.Submit(func() {
		cancel()
	}).Wait()

	// If the context is canceled during the task execution, the task should return the context error.
	assert.Equal(t, context.Canceled, err)

	err = pool.Submit(func() {}).Wait()

	// If the context is canceled, the pool should be stopped and the task should return the pool stopped error.
	assert.Equal(t, ErrPoolStopped, err)
	assert.True(t, pool.Stopped())

	err = pool.Go(func() {})

	assert.Equal(t, ErrPoolStopped, err)

	pool.StopAndWait()

	err = pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = pool.Go(func() {})

	assert.Equal(t, ErrPoolStopped, err)
}
