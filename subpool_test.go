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
	assert.Equal(t, uint64(3), pool.FailedTasks())
	assert.Equal(t, uint64(17), pool.SuccessfulTasks())
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

	assert.PanicsWithError(t, "maxConcurrency must be greater or equal to 0", func() {
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
