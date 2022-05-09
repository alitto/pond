package pond_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond"
)

func TestGroupSubmit(t *testing.T) {

	pool := pond.New(5, 1000)
	assertEqual(t, 0, pool.RunningWorkers())

	// Submit groups of tasks
	var doneCount, taskCount int32
	var groups []*pond.TaskGroup
	for i := 0; i < 5; i++ {
		group := pool.Group()
		for j := 0; j < i+5; j++ {
			group.Submit(func() {
				time.Sleep(1 * time.Millisecond)
				atomic.AddInt32(&doneCount, 1)
			})
			taskCount++
		}
		groups = append(groups, group)
	}

	// Wait for all groups to complete
	for _, group := range groups {
		group.Wait()
	}

	assertEqual(t, int32(taskCount), atomic.LoadInt32(&doneCount))
}

func TestGroupContext(t *testing.T) {

	pool := pond.New(3, 100)
	assertEqual(t, 0, pool.RunningWorkers())

	// Submit a group of tasks
	var doneCount, startedCount int32
	group, ctx := pool.GroupContext(context.Background())
	for i := 0; i < 10; i++ {
		group.Submit(func() error {
			atomic.AddInt32(&startedCount, 1)

			select {
			case <-time.After(5 * time.Millisecond):
				atomic.AddInt32(&doneCount, 1)
			case <-ctx.Done():
			}

			return nil
		})
	}

	err := group.Wait()
	assertEqual(t, nil, err)
	assertEqual(t, int32(10), atomic.LoadInt32(&startedCount))
	assertEqual(t, int32(10), atomic.LoadInt32(&doneCount))
}

func TestGroupContextWithError(t *testing.T) {

	pool := pond.New(1, 100)
	assertEqual(t, 0, pool.RunningWorkers())

	expectedErr := errors.New("Something went wrong")

	// Submit a group of tasks
	var doneCount, startedCount int32
	group, ctx := pool.GroupContext(context.Background())
	for i := 0; i < 10; i++ {
		n := i
		group.Submit(func() error {
			atomic.AddInt32(&startedCount, 1)

			// Task number 5 fails
			if n == 4 {
				time.Sleep(10 * time.Millisecond)
				return expectedErr
			}

			select {
			case <-time.After(5 * time.Millisecond):
				atomic.AddInt32(&doneCount, 1)
			case <-ctx.Done():
			}

			return nil
		})
	}

	err := group.Wait()
	assertEqual(t, expectedErr, err)

	pool.StopAndWait()

	assertEqual(t, int32(5), atomic.LoadInt32(&startedCount))
	assertEqual(t, int32(4), atomic.LoadInt32(&doneCount))
}
