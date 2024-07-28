package pond

import (
	"context"
	"errors"
	"fmt"
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

func TestWorkerPoolSubmit(t *testing.T) {

	pool := NewPool[any](context.Background(), 10000)

	var taskCount int = 100000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	pool.Stop().Wait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}

func TestPoolSubmitWait(t *testing.T) {

	pool := NewPool[int](context.Background(), 10000)

	task := pool.Submit(func() int {
		return 5
	})

	out, err := task.Get()

	assertEqual(t, nil, err)
	assertEqual(t, 5, out)
}

func TestPoolSubmitTaskWithPanic(t *testing.T) {

	pool := NewPool[int](context.Background(), 10000)

	task := pool.Submit(func() int {
		panic("dummy panic")
	})

	out, err := task.Get()

	assertEqual(t, true, errors.Is(err, ErrPanic))
	assertEqual(t, 0, out)
}

func TestPoolGo(t *testing.T) {

	pool := NewPool[any](context.Background(), 10000)

	done := make(chan int, 1)
	pool.Go(func() {
		done <- 3
	})

	pool.Stop().Wait()

	assertEqual(t, 3, <-done)
}
