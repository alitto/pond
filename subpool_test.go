package pond

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestSubpool(t *testing.T) {
	taskDuration := 10 * time.Millisecond
	taskCount := 10
	maxConcurrency := 5

	pool := NewPool(10)
	subpool := pool.NewSubpool(maxConcurrency)
	subsubpool := subpool.NewSubpool(1)

	completed := make(chan struct{})
	var subpoolElapsedTime time.Duration
	var subsubpoolElapsedTime time.Duration

	go func() {
		start := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			subpool.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
		subpoolElapsedTime = time.Since(start)
		completed <- struct{}{}
	}()

	go func() {
		start := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			subsubpool.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
		subsubpool.Stop().Wait()
		subsubpoolElapsedTime = time.Since(start)
		completed <- struct{}{}
	}()

	<-completed
	<-completed

	assert.True(t, subpoolElapsedTime >= time.Duration(taskCount/maxConcurrency)*taskDuration)
	assert.True(t, subsubpoolElapsedTime >= time.Duration(taskCount/1)*taskDuration)
}

func TestSubpoolStopAndWait(t *testing.T) {
	taskDuration := 1 * time.Microsecond
	taskCount := 100000
	maxConcurrency := 100

	subpool := NewPool(1000).NewSubpool(maxConcurrency)

	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		subpool.Submit(func() {
			time.Sleep(taskDuration)
			executedCount.Add(1)
		})
	}

	subpool.StopAndWait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
}

func TestSubpoolMetrics(t *testing.T) {
	pool := NewPool(10)
	subpool := pool.NewSubpool(5)

	sampleErr := errors.New("sample error")

	for i := 0; i < 20; i++ {
		i := i
		if i%2 == 0 {
			subpool.SubmitErr(func() error {
				if i%4 == 0 {
					return sampleErr
				}

				return nil
			})
		} else {
			pool.SubmitErr(func() error {
				if i%3 == 0 {
					return sampleErr
				}

				return nil
			})
		}
	}

	subpool.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, uint64(20), pool.SubmittedTasks())
	assert.Equal(t, uint64(20), pool.CompletedTasks())
	assert.Equal(t, uint64(8), pool.FailedTasks())
	assert.Equal(t, uint64(12), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.WaitingTasks())

	assert.Equal(t, uint64(10), subpool.SubmittedTasks())
	assert.Equal(t, uint64(10), subpool.CompletedTasks())
	assert.Equal(t, uint64(5), subpool.FailedTasks())
	assert.Equal(t, uint64(5), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())

}

func TestSubpoolStop(t *testing.T) {
	pool := NewPool(10)
	subpool := pool.NewSubpool(5)

	var executedCount atomic.Int64

	subpool.Submit(func() {
		executedCount.Add(1)
	})

	subpool.StopAndWait()

	pool.Submit(func() {
		executedCount.Add(1)
	}).Wait()

	assert.Equal(t, int64(2), executedCount.Load())

	err := subpool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	pool.StopAndWait()

	err = pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestSubpoolMaxConcurrency(t *testing.T) {
	pool := NewPool(10)

	assert.PanicsWithError(t, "maxConcurrency must be greater than 0", func() {
		pool.NewSubpool(-1)
	})

	assert.PanicsWithError(t, "maxConcurrency cannot be greater than the parent pool's maxConcurrency (10)", func() {
		pool.NewSubpool(11)
	})

	subpool := pool.NewSubpool(0)

	assert.Equal(t, 10, subpool.MaxConcurrency())
}

func TestSubpoolStoppedAfterCancel(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(10, WithContext(ctx))
	subpool := pool.NewSubpool(5)

	cancel()

	time.Sleep(1 * time.Millisecond)

	err := pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = subpool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestSubpoolWithDifferentLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewPool(7, WithContext(ctx))

	subpool1 := pool.NewSubpool(1)
	subpool2 := pool.NewSubpool(2)
	subpool3 := pool.NewSubpool(3)

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	var task = func() func() {
		return func() {
			taskStarted <- struct{}{}
			<-taskWait
		}
	}

	// Submit tasks to subpool1 and wait for 1 task to start
	for i := 0; i < 10; i++ {
		subpool1.Submit(task())
	}
	<-taskStarted

	// Submit tasks to subpool2 and wait for 2 tasks to start
	for i := 0; i < 10; i++ {
		subpool2.Submit(task())
	}
	<-taskStarted
	<-taskStarted

	// Submit tasks to subpool3 and wait for 3 tasks to start
	for i := 0; i < 10; i++ {
		subpool3.Submit(task())
	}
	<-taskStarted
	<-taskStarted
	<-taskStarted

	// Submit tasks to the main pool and wait for 1 to start
	for i := 0; i < 10; i++ {
		pool.Submit(task())
	}
	<-taskStarted

	// Verify concurrency of each pool
	assert.Equal(t, int64(1), subpool1.RunningWorkers())
	assert.Equal(t, int64(2), subpool2.RunningWorkers())
	assert.Equal(t, int64(3), subpool3.RunningWorkers())
	assert.Equal(t, int64(7), pool.RunningWorkers())

	assert.Equal(t, uint64(0), subpool1.CompletedTasks())
	assert.Equal(t, uint64(0), subpool2.CompletedTasks())
	assert.Equal(t, uint64(0), subpool3.CompletedTasks())
	assert.Equal(t, uint64(0), pool.CompletedTasks())

	// Cancel the context to abort pending tasks
	cancel()

	// Unblock all running tasks
	close(taskWait)

	subpool1.StopAndWait()
	subpool2.StopAndWait()
	subpool3.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, uint64(1), subpool1.CompletedTasks())
	assert.Equal(t, uint64(2), subpool2.CompletedTasks())
	assert.Equal(t, uint64(3), subpool3.CompletedTasks())
	assert.Equal(t, uint64(7), pool.CompletedTasks())
}

func TestSubpoolWithOverlappingConcurrency(t *testing.T) {
	taskDuration := 1 * time.Millisecond

	pool := NewPool(1)
	subpool := pool.NewSubpool(1)

	var executedCount atomic.Int64

	subpool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	pool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	subpool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	subpool.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, int64(3), executedCount.Load())
}

func TestSubpoolWithQueueSizeOverride(t *testing.T) {
	pool := NewPool(10, WithQueueSize(10))

	subpool := pool.NewSubpool(1, WithQueueSize(2), WithNonBlocking(true))

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	var task = func() func() {
		return func() {
			taskStarted <- struct{}{}
			<-taskWait
		}
	}

	// Submit tasks to subpool and wait for it to start
	subpool.Submit(task())
	<-taskStarted

	// Submit more tasks to fill up the queue
	for i := 0; i < 10; i++ {
		subpool.Submit(task())
	}

	// 7 tasks should have been discarded
	assert.Equal(t, int64(1), subpool.RunningWorkers())
	assert.Equal(t, uint64(3), subpool.SubmittedTasks())

	// Unblock all running tasks
	close(taskWait)

	subpool.StopAndWait()
	pool.StopAndWait()
}

func TestSubpoolResize(t *testing.T) {

	parentPool := NewPool(10, WithQueueSize(10))

	pool := parentPool.NewSubpool(1)

	assert.Equal(t, 1, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

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
	assert.Equal(t, int64(1), parentPool.RunningWorkers())

	// Increase max concurrency to 3
	pool.Resize(3)
	assert.Equal(t, 3, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

	// Unblock 3 more tasks
	for i := 0; i < 3; i++ {
		taskStarted <- struct{}{}
	}

	// Verify 3 tasks are running and 7 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(7), pool.WaitingTasks())
	assert.Equal(t, int64(3), pool.RunningWorkers())
	assert.Equal(t, int64(3), parentPool.RunningWorkers())

	// Decrease max concurrency to 1
	pool.Resize(2)
	assert.Equal(t, 2, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

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
	assert.Equal(t, int64(2), parentPool.RunningWorkers())

	close(taskWait)

	pool.Stop().Wait()
}
