package pond

import (
	"context"
	"errors"
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

func TestPoolGo(t *testing.T) {

	pool := NewPool(10000)

	done := make(chan int, 1)
	pool.Go(func() {
		done <- 3
	})

	pool.Stop().Wait()

	assert.Equal(t, 3, <-done)
}

func TestPoolWithContextCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := WithContext(ctx).NewPool(10)

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
