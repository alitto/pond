package pond

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitVoid(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	taskCtx := SubmitVoid(pool, func(_ context.Context) {
		time.Sleep(1 * time.Millisecond)
	})

	<-taskCtx.Done()

	pool.StopAndWait()

	assertEqual(t, nil, taskCtx.Wait())
}

func TestSubmitVoidCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(ctx, 10000)

	taskCtx := SubmitVoid(pool, func(ctx context.Context) {
		cancel()
		select {
		case <-ctx.Done():
		case <-time.NewTimer(1 * time.Millisecond).C:
		}
	})

	<-taskCtx.Done()

	assertEqual(t, context.Canceled, taskCtx.Wait())
}

func TestSubmitVoidWithContextCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(ctx, 10000)

	sampleErr := fmt.Errorf("sample error")

	err := SubmitVoid(pool, func(ctx context.Context) error {
		// Parent context is canceled while the task is executing
		cancel()
		return sampleErr
	}).Wait()

	assertEqual(t, context.Canceled, err)
}

func TestSubmitVoidWithError(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(ctx, 10000)

	sampleErr := fmt.Errorf("sample error")

	err := SubmitVoid(pool, func(ctx context.Context) error {
		return sampleErr
	}).Wait()

	// Context is canceled after the task was executed, so it should be ignored
	cancel()

	assertEqual(t, sampleErr, err)
}

func TestSubmitWait(t *testing.T) {

	pool := NewPool(context.Background(), 1000)

	var taskCount int = 10
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		err := SubmitVoid(pool, func(_ context.Context) {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		}).Wait()
		assertEqual(t, nil, err)
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestSubmitWithResult(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	var taskCount int = 10000
	var executedCount atomic.Int64
	var taskContexts = make([]ResultContext[int], 0)

	for i := 0; i < taskCount; i++ {
		n := i
		taskCtx := Submit[int](pool, func(_ context.Context) (int, error) {
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
	}

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestSubmitWithResultInt(t *testing.T) {

	pool := NewPool(context.Background(), 10000)

	result, err := Submit[int](pool, func(_ context.Context) (int, error) {
		time.Sleep(1 * time.Millisecond)
		return 5, nil
	}).Wait()

	pool.StopAndWait()

	assertEqual(t, 5, result)
	assertEqual(t, nil, err)
}
