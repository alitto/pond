package pond

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func assertEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

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

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestTypedPoolSubmitAndGet(t *testing.T) {

	pool := NewTypedPool[int](10000)
	defer pool.StopAndWait()

	task := pool.Submit(func() int {
		return 5
	})

	out, err := task.Get()

	assertEqual(t, nil, err)
	assertEqual(t, 5, out)
}

func TestTypedPoolSubmitTaskWithPanic(t *testing.T) {

	pool := NewTypedPool[int](10000)

	task := pool.Submit(func() int {
		panic("dummy panic")
	})

	out, err := task.Get()

	assertEqual(t, true, errors.Is(err, ErrPanic))
	assertEqual(t, "task panicked: dummy panic", err.Error())
	assertEqual(t, 0, out)
}

func TestPoolGo(t *testing.T) {

	pool := NewPool(10000)

	done := make(chan int, 1)
	pool.Go(func() {
		done <- 3
	})

	pool.Stop().Wait()

	assertEqual(t, 3, <-done)
}

func TestPoolWithContextCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(10, WithContext(ctx))

	assertEqual(t, int64(0), pool.RunningWorkers())
	assertEqual(t, uint64(0), pool.SubmittedTasks())

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	assertEqual(t, int64(10), pool.RunningWorkers())
	assertEqual(t, uint64(taskCount), pool.SubmittedTasks())

	// Cancel the context after 5ms
	time.Sleep(5 * time.Millisecond)
	cancel()

	pool.Stop().Wait()

	assertEqual(t, true, executedCount.Load() < int64(taskCount))
}

func TestPoolMetrics(t *testing.T) {

	pool := NewPool(100)

	// Assert counters
	assertEqual(t, int64(0), pool.RunningWorkers())
	assertEqual(t, uint64(0), pool.SubmittedTasks())
	assertEqual(t, uint64(0), pool.CompletedTasks())
	assertEqual(t, uint64(0), pool.FailedTasks())
	assertEqual(t, uint64(0), pool.SuccessfulTasks())
	assertEqual(t, uint64(0), pool.WaitingTasks())

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		n := i
		pool.Submit(func() error {
			executedCount.Add(1)
			if n%2 == 0 {
				return nil
			}
			return errors.New("sample error")
		})
	}

	pool.Stop().Wait()

	assertEqual(t, int64(taskCount), executedCount.Load())
	assertEqual(t, int64(0), pool.RunningWorkers())
	assertEqual(t, uint64(taskCount), pool.SubmittedTasks())
	assertEqual(t, uint64(taskCount), pool.CompletedTasks())
	assertEqual(t, uint64(taskCount/2), pool.FailedTasks())
	assertEqual(t, uint64(taskCount/2), pool.SuccessfulTasks())
}
