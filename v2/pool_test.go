package pond

import (
	"context"
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

	pool := NewPool(context.Background(), 10000)

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	fmt.Printf("Submitted %d tasks\n", taskCount)

	pool.StopAndWait()

	assertEqual(t, int64(taskCount), executedCount.Load())
}
