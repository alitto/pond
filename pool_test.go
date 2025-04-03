package pond

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sync"
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
	assert.Equal(t, int64(0), pool.RunningWorkers())
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
	assert.Equal(t, uint64(0), pool.DroppedTasks())
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
	assert.Equal(t, uint64(0), pool.DroppedTasks())
}

func TestPoolSubmitOnStoppedPool(t *testing.T) {

	pool := newPool(100, nil)

	pool.Submit(func() {})

	pool.StopAndWait()

	err := pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = pool.Go(func() {})

	assert.Equal(t, ErrPoolStopped, err)
	assert.Equal(t, true, pool.Stopped())
}

func TestNewPoolWithInvalidMaxConcurrency(t *testing.T) {
	assert.PanicsWithError(t, "maxConcurrency must be greater than or equal to 0", func() {
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

func TestPoolWithQueueSize(t *testing.T) {

	pool := NewPool(1, WithQueueSize(10))

	assert.Equal(t, 10, pool.QueueSize())
	assert.Equal(t, false, pool.NonBlocking())

	var taskCount int = 50

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
		})
	}

	pool.Stop().Wait()

	assert.Equal(t, uint64(taskCount), pool.SubmittedTasks())
	assert.Equal(t, uint64(taskCount), pool.CompletedTasks())
}

func TestPoolWithQueueSizeAndNonBlocking(t *testing.T) {

	pool := NewPool(10, WithQueueSize(10), WithNonBlocking(true))

	assert.Equal(t, 10, pool.QueueSize())
	assert.Equal(t, true, pool.NonBlocking())

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			taskStarted <- struct{}{}
			<-taskWait
		})
	}

	// Wait for 10 tasks to start
	for i := 0; i < 10; i++ {
		<-taskStarted
	}

	assert.Equal(t, int64(10), pool.RunningWorkers())
	assert.Equal(t, uint64(10), pool.SubmittedTasks())
	assert.Equal(t, uint64(0), pool.WaitingTasks())

	// Saturate the queue
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Submit a task that should be dropped
	task := pool.Submit(func() {})

	// Unblock tasks
	close(taskWait)

	assert.Equal(t, ErrQueueFull, task.Wait())
	assert.Equal(t, uint64(1), pool.DroppedTasks())

	pool.Stop().Wait()
}

func TestPoolResize(t *testing.T) {

	pool := NewPool(1, WithQueueSize(10))

	assert.Equal(t, 1, pool.MaxConcurrency())

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{}, 10)

	// Submit 10 tasks
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			<-taskStarted
			<-taskWait
		})
	}

	// Unblock 3 tasks
	for i := 0; i < 3; i++ {
		taskStarted <- struct{}{}
	}

	// Verify only 1 task is running and 9 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(9), pool.WaitingTasks())
	assert.Equal(t, int64(1), pool.RunningWorkers())

	// Increase max concurrency to 3
	pool.Resize(3)
	assert.Equal(t, 3, pool.MaxConcurrency())

	// Unblock 3 more tasks
	for i := 0; i < 3; i++ {
		taskStarted <- struct{}{}
	}

	// Verify 3 tasks are running and 7 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(7), pool.WaitingTasks())
	assert.Equal(t, int64(3), pool.RunningWorkers())

	// Decrease max concurrency to 1
	pool.Resize(2)
	assert.Equal(t, 2, pool.MaxConcurrency())

	// Complete the 3 running tasks
	for i := 0; i < 3; i++ {
		taskWait <- struct{}{}
	}

	// Unblock all remaining tasks
	for i := 0; i < 4; i++ {
		taskStarted <- struct{}{}
	}

	// Ensure 2 tasks are running and 5 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(5), pool.WaitingTasks())
	assert.Equal(t, int64(2), pool.RunningWorkers())

	close(taskWait)

	pool.Stop().Wait()
}

func TestPoolResizeWithZeroMaxConcurrency(t *testing.T) {
	pool := NewPool(10)

	pool.Resize(0)

	assert.Equal(t, math.MaxInt, pool.MaxConcurrency())
}

func TestPoolResizeWithNegativeMaxConcurrency(t *testing.T) {
	assert.PanicsWithError(t, "maxConcurrency must be greater than or equal to 0", func() {
		NewPool(10).Resize(-1)
	})
}

func TestPoolSubmitWhileStopping(t *testing.T) {

	pool := NewPool(10)

	task := pool.Submit(func() {
		// Wait until the pool is stopped
		for !pool.Stopped() {
			time.Sleep(1 * time.Millisecond)
		}

		assert.Equal(t, true, pool.Stopped())

		// Submit a task to the pool while it is stopping
		err := pool.Submit(func() {}).Wait()

		assert.Equal(t, ErrPoolStopped, err)

		err = pool.Go(func() {})

		assert.Equal(t, ErrPoolStopped, err)

		err = pool.SubmitErr(func() error {
			return errors.New("sample error")
		}).Wait()

		assert.Equal(t, ErrPoolStopped, err)
	})

	stopErr := pool.Stop().Wait()

	assert.Equal(t, nil, task.Wait())

	assert.Equal(t, nil, stopErr)

	err := pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = pool.Go(func() {})

	assert.Equal(t, ErrPoolStopped, err)

	err = pool.SubmitErr(func() error {
		return errors.New("sample error")
	}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	pool.StopAndWait()
}

// Test that submitting tasks while the pool is stopping doesn't lead to a data race.
// Author: https://github.com/korotin
func TestPoolSubmitWhileStoppingHasNoRace(t *testing.T) {
	pool := NewPool(0)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(500 * time.Microsecond)
		pool.StopAndWait()
	}()

	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			pool.Submit(func() {
				time.Sleep(10 * time.Millisecond)
			})
		}()
	}

	wg.Wait()
}

func TestPoolTrySubmit(t *testing.T) {
	pool := NewPool(1, WithQueueSize(1))

	completeFirstTask := make(chan struct{})

	// Test successful submission
	task, ok := pool.TrySubmit(func() {
		completeFirstTask <- struct{}{}
	})
	assert.True(t, ok)

	// Test submission to queue
	task2, ok := pool.TrySubmit(func() {})
	assert.True(t, ok)

	// Test failed submission when queue is full
	task3, ok := pool.TrySubmit(func() {})
	assert.True(t, !ok)

	<-completeFirstTask

	// Test submission to stopped pool
	pool.StopAndWait()
	task4, ok := pool.TrySubmit(func() {})
	assert.True(t, !ok)
	assert.Equal(t, ErrPoolStopped, task4.Wait())

	// Verify tasks completed successfully
	assert.Equal(t, nil, task.Wait())
	assert.Equal(t, nil, task2.Wait())
	assert.Equal(t, ErrQueueFull, task3.Wait())

	// Verify metrics
	assert.Equal(t, uint64(3), pool.SubmittedTasks())
	assert.Equal(t, uint64(2), pool.CompletedTasks())
	assert.Equal(t, uint64(2), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.FailedTasks())
	assert.Equal(t, uint64(1), pool.DroppedTasks())
}

func TestPoolTrySubmitErr(t *testing.T) {
	pool := NewPool(1, WithQueueSize(1))

	completeFirstTask := make(chan struct{})

	// Test successful submission with no error
	task, ok := pool.TrySubmitErr(func() error {
		completeFirstTask <- struct{}{}
		return nil
	})
	assert.True(t, ok)

	// Test submission to queue with error
	task2, ok := pool.TrySubmitErr(func() error {
		return errors.New("sample error")
	})
	assert.True(t, ok)

	// Test failed submission when queue is full
	task3, ok := pool.TrySubmitErr(func() error {
		return nil
	})
	assert.True(t, !ok)

	<-completeFirstTask

	// Test submission to stopped pool
	pool.StopAndWait()
	task4, ok := pool.TrySubmitErr(func() error {
		return nil
	})
	assert.True(t, !ok)
	assert.Equal(t, ErrPoolStopped, task4.Wait())

	// Verify tasks completed successfully
	assert.Equal(t, nil, task.Wait())
	assert.Equal(t, "sample error", task2.Wait().Error())
	assert.Equal(t, ErrQueueFull, task3.Wait())

	// Verify metrics
	assert.Equal(t, uint64(3), pool.SubmittedTasks())
	assert.Equal(t, uint64(2), pool.CompletedTasks())
	assert.Equal(t, uint64(1), pool.SuccessfulTasks())
	assert.Equal(t, uint64(1), pool.FailedTasks())
	assert.Equal(t, uint64(1), pool.DroppedTasks())
}
