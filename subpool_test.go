package pond

import (
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
	subpool := pool.Subpool(maxConcurrency)
	subsubpool := subpool.Subpool(1)

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

	subpool := NewPool(1000).Subpool(maxConcurrency)

	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		subpool.Submit(func() {
			time.Sleep(taskDuration)
			executedCount.Add(1)
		})
	}

	subpool.Stop().Wait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
}

func TestSubpoolMetrics(t *testing.T) {
	pool := NewPool(10)
	subpool := pool.Subpool(5)

	sampleErr := errors.New("sample error")

	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			subpool.Submit(func() error {
				if i%4 == 0 {
					return sampleErr
				}

				return nil
			})
		} else {
			pool.Submit(func() error {
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
