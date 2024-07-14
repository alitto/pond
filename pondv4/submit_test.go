package pondv4

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

/*
func TestSubmit(t *testing.T) {

		pool := NewPool(context.Background(), 10000)

		result := <-Submit(pool, func(_ context.Context) {
			time.Sleep(1 * time.Millisecond)
		})

		pool.StopAndWait()

		assertEqual(t, result.Result(), nil)
		assertEqual(t, result.Err(), nil)
	}
*/
func TestSubmit(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	taskCtx := Submit(pool, func(_ context.Context) {
		time.Sleep(1 * time.Millisecond)
	})

	<-taskCtx.Done()

	pool.StopAndWait()

	assertEqual(t, nil, taskCtx.Err())
}

func TestSubmitWithContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(ctx, 10000)

	taskCtx := Submit(pool, func(ctx context.Context) {
		cancel()
		select {
		case <-ctx.Done():
		case <-time.NewTimer(1 * time.Millisecond).C:
		}
	})

	<-taskCtx.Done()

	assertEqual(t, context.Canceled, taskCtx.Err())
}

func TestSubmitWaitWithContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(ctx, 10000)

	err := Submit(pool, func(ctx context.Context) {
		cancel()
	}).Wait()

	assertEqual(t, context.Canceled, err)
}

func TestSubmitWait(t *testing.T) {

	pool := NewPool(context.Background(), 1000)

	var taskCount int = 10
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		err := Submit(pool, func(_ context.Context) {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		}).Wait()
		fmt.Printf("Err is %v\n", err)
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestSubmitAndGet(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	var taskCount int = 10000
	var executedCount atomic.Int64
	var taskContexts = make([]ResultTaskContext[int], 0)

	for i := 0; i < taskCount; i++ {
		n := i
		taskCtx := SubmitAndGet[int](pool, func(_ context.Context) (int, error) {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
			return n, nil
		})
		taskContexts = append(taskContexts, taskCtx)
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	// Wait for all tasks to complete
	for _, tc := range taskContexts {
		<-tc.Done()
		fmt.Printf("Result is %d, %v\n", tc.Result(), tc.Err())
	}

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestSubmitWaitAndGet(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	var taskCount int = 10
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		n := i
		result, err := SubmitAndGet[int](pool, func(_ context.Context) (int, error) {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
			return n, nil
		}).Wait()
		fmt.Printf("Result is %d - err is %v\n", result, err)
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}
