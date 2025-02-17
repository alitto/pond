package pond

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestResultTaskGroupWait(t *testing.T) {

	pool := NewResultPool[int](10)

	group := pool.NewGroup()

	for i := 0; i < 5; i++ {
		i := i
		group.Submit(func() int {
			return i
		})
	}

	outputs, err := group.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(outputs))
	assert.Equal(t, 0, outputs[0])
	assert.Equal(t, 1, outputs[1])
	assert.Equal(t, 2, outputs[2])
	assert.Equal(t, 3, outputs[3])
	assert.Equal(t, 4, outputs[4])
}

func TestResultTaskGroupWaitWithError(t *testing.T) {

	pool := NewResultPool[int](1)

	group := pool.NewGroup()

	sampleErr := errors.New("sample error")

	for i := 0; i < 5; i++ {
		i := i
		if i == 3 {
			group.SubmitErr(func() (int, error) {
				return 0, sampleErr
			})
		} else {
			group.SubmitErr(func() (int, error) {
				return i, nil
			})
		}
	}

	outputs, err := group.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 5, len(outputs))
	assert.Equal(t, 0, outputs[0])
	assert.Equal(t, 1, outputs[1])
	assert.Equal(t, 2, outputs[2])
	assert.Equal(t, 0, outputs[3]) // This task returned an error
	assert.Equal(t, 0, outputs[4]) // This task was not executed
}

func TestResultTaskGroupWaitWithErrorInLastTask(t *testing.T) {

	group := NewResultPool[int](10).
		NewGroup()

	sampleErr := errors.New("sample error")

	group.SubmitErr(func() (int, error) {
		return 1, nil
	})

	time.Sleep(10 * time.Millisecond)

	group.SubmitErr(func() (int, error) {
		return 0, sampleErr
	})

	outputs, err := group.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 2, len(outputs))
	assert.Equal(t, 1, outputs[0])
	assert.Equal(t, 0, outputs[1])
}

func TestResultTaskGroupWaitWithMultipleErrors(t *testing.T) {

	pool := NewResultPool[int](10)

	group := pool.NewGroup()

	sampleErr := errors.New("sample error")

	for i := 0; i < 5; i++ {
		i := i
		group.SubmitErr(func() (int, error) {
			if i%2 == 0 {
				time.Sleep(10 * time.Millisecond)
				return 0, sampleErr
			}
			return i, nil
		})
	}

	outputs, err := group.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 5, len(outputs))
	assert.Equal(t, 0, outputs[0])
	assert.Equal(t, 1, outputs[1])
	assert.Equal(t, 0, outputs[2])
	assert.Equal(t, 3, outputs[3])
	assert.Equal(t, 0, outputs[4])
}

func TestResultTaskGroupWaitWithContextCanceledAndOngoingTasks(t *testing.T) {
	pool := NewResultPool[string](1)

	ctx, cancel := context.WithCancel(context.Background())

	group := pool.NewGroupContext(ctx)

	group.Submit(func() string {
		cancel() // cancel the context after the first task is started
		time.Sleep(10 * time.Millisecond)
		return "output1"
	})

	group.Submit(func() string {
		time.Sleep(10 * time.Millisecond)
		return "output2"
	})

	results, err := group.Wait()

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, int(2), len(results))
	assert.Equal(t, "", results[0])
	assert.Equal(t, "", results[1])
}

func TestTaskGroupWaitWithContextCanceledAndOngoingTasks(t *testing.T) {
	pool := NewPool(1)

	var executedCount atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())

	group := pool.NewGroupContext(ctx)

	group.Submit(func() {
		cancel() // cancel the context after the first task is started
		time.Sleep(10 * time.Millisecond)
		executedCount.Add(1)
	})

	group.Submit(func() {
		time.Sleep(10 * time.Millisecond)
		executedCount.Add(1)
	})

	err := group.Wait()

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, int32(1), executedCount.Load())
}

func TestTaskGroupWithStoppedPool(t *testing.T) {

	pool := NewPool(100)

	pool.StopAndWait()

	err := pool.NewGroup().Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestTaskGroupWithContextCanceled(t *testing.T) {

	pool := NewPool(100)

	group := pool.NewGroup()

	ctx, cancel := context.WithCancel(context.Background())

	taskStarted := make(chan struct{})

	task := group.SubmitErr(func() error {
		taskStarted <- struct{}{}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	<-taskStarted
	cancel()

	err := task.Wait()

	assert.Equal(t, context.Canceled, err)
}

func TestTaskGroupWithNoTasks(t *testing.T) {

	group := NewResultPool[int](10).
		NewGroup()

	results, err := group.Submit().Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(results))

	results, err = group.SubmitErr().Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(results))
}

func TestTaskGroupCanceledShouldSkipRemainingTasks(t *testing.T) {

	pool := NewPool(1)

	group := pool.NewGroup()

	var executedCount atomic.Int32
	sampleErr := errors.New("sample error")

	group.Submit(func() {
		executedCount.Add(1)
	})

	group.SubmitErr(func() error {
		time.Sleep(10 * time.Millisecond)
		return sampleErr
	})

	group.Submit(func() {
		executedCount.Add(1)
	})

	err := group.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, int32(1), executedCount.Load())
}

func TestTaskGroupWithCustomContext(t *testing.T) {
	pool := NewPool(1)

	ctx, cancel := context.WithCancel(context.Background())

	group := pool.NewGroupContext(ctx)

	var executedCount atomic.Int32

	group.Submit(func() {
		executedCount.Add(1)
	})
	group.Submit(func() {
		executedCount.Add(1)
		cancel()
	})
	group.Submit(func() {
		executedCount.Add(1)
	})

	err := group.Wait()

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, struct{}{}, <-group.Done())
	assert.Equal(t, int32(2), executedCount.Load())
}

func TestTaskGroupStop(t *testing.T) {
	pool := NewPool(1)

	group := pool.NewGroup()

	var executedCount atomic.Int32

	group.Submit(func() {
		executedCount.Add(1)
	})
	group.Submit(func() {
		executedCount.Add(1)
		group.Stop()
	})
	group.Submit(func() {
		executedCount.Add(1)
	})

	err := group.Wait()

	assert.Equal(t, ErrGroupStopped, err)
	assert.Equal(t, struct{}{}, <-group.Done())
	assert.Equal(t, int32(2), executedCount.Load())
}

func TestTaskGroupDone(t *testing.T) {
	pool := NewPool(10)

	group := pool.NewGroup()

	var executedCount atomic.Int32

	for i := 0; i < 5; i++ {
		group.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			executedCount.Add(1)
		})
	}

	<-group.Done()

	assert.Equal(t, int32(5), executedCount.Load())
}

func TestTaskGroupMetrics(t *testing.T) {
	pool := NewPool(1)

	group := pool.NewGroup()

	for i := 0; i < 9; i++ {
		group.Submit(func() {
			time.Sleep(1 * time.Millisecond)
		})
	}

	// The last task will return an error
	sampleErr := errors.New("sample error")
	group.SubmitErr(func() error {
		time.Sleep(1 * time.Millisecond)
		return sampleErr
	})

	err := group.Wait()

	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, uint64(10), pool.SubmittedTasks())
	assert.Equal(t, uint64(9), pool.SuccessfulTasks())
	assert.Equal(t, uint64(1), pool.FailedTasks())
}

func TestTaskGroupMetricsWithCancelledContext(t *testing.T) {
	pool := NewPool(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group := pool.NewGroupContext(ctx)

	for i := 0; i < 10; i++ {
		i := i
		group.Submit(func() {
			time.Sleep(20 * time.Millisecond)
			if i == 4 {
				cancel()
			}
		})
	}
	err := group.Wait()

	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, err, context.Canceled)
	assert.Equal(t, uint64(10), pool.SubmittedTasks())
	assert.Equal(t, uint64(5), pool.SuccessfulTasks())
	assert.Equal(t, uint64(5), pool.FailedTasks())
}

func TestTaskGroupWaitingTasks(t *testing.T) {
	// Create a pool with limited concurrency
	pool := NewPool(10)

	// Create a task group
	group := pool.NewGroup()

	start := make(chan struct{})
	end := make(chan struct{})

	// Submit 20 tasks
	for i := 0; i < 20; i++ {
		group.Submit(func() {
			<-start
			<-end
		})
	}

	// Start half of the tasks
	for i := 0; i < 10; i++ {
		start <- struct{}{}
	}

	assert.Equal(t, int64(10), pool.RunningWorkers())
	assert.Equal(t, uint64(10), pool.WaitingTasks())

	// Wait for all tasks to complete
	close(start)
	close(end)
	group.Wait()
}
