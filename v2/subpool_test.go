package pond

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubpool(t *testing.T) {
	taskDuration := 10 * time.Millisecond
	taskCount := 10
	maxConcurrency := 5

	pool := NewPool(context.Background(), 10)
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

	assertEqual(t, true, subpoolElapsedTime >= time.Duration(taskCount/maxConcurrency)*taskDuration)
	assertEqual(t, true, subsubpoolElapsedTime >= time.Duration(taskCount/1)*taskDuration)
}

func TestSubpoolStopAndWait(t *testing.T) {
	taskDuration := 1 * time.Microsecond
	taskCount := 100000
	maxConcurrency := 100

	subpool := NewPool(context.Background(), 1000).Subpool(maxConcurrency)

	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		subpool.Submit(func() {
			time.Sleep(taskDuration)
			executedCount.Add(1)
		})
	}

	subpool.Stop().Wait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}
