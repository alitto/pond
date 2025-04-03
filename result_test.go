package pond

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestResultPoolSubmitAndWait(t *testing.T) {

	pool := NewResultPool[int](1000)
	defer pool.StopAndWait()

	task := pool.Submit(func() int {
		return 5
	})

	output, err := task.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, output)
}

func TestResultPoolSubmitTaskWithPanic(t *testing.T) {

	pool := NewResultPool[int](1000)

	task := pool.Submit(func() int {
		panic("dummy panic")
	})

	output, err := task.Wait()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.True(t, strings.HasPrefix(err.Error(), "task panicked: dummy panic"))
	assert.Equal(t, 0, output)
}

func TestResultPoolMetrics(t *testing.T) {

	pool := NewResultPool[int](1000)

	// Assert counters
	assert.Equal(t, int64(0), pool.RunningWorkers())
	assert.Equal(t, uint64(0), pool.SubmittedTasks())
	assert.Equal(t, uint64(0), pool.CompletedTasks())
	assert.Equal(t, uint64(0), pool.FailedTasks())
	assert.Equal(t, uint64(0), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.WaitingTasks())

	var taskCount int = 10000
	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		i := i
		pool.SubmitErr(func() (int, error) {
			executedCount.Add(1)
			if i%2 == 0 {
				return i, nil
			}
			return 0, errors.New("sample error")
		})
	}

	pool.Stop().Wait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
	assert.Equal(t, int64(0), pool.RunningWorkers())
	assert.Equal(t, uint64(taskCount), pool.SubmittedTasks())
	assert.Equal(t, uint64(taskCount), pool.CompletedTasks())
	assert.Equal(t, uint64(taskCount/2), pool.FailedTasks())
	assert.Equal(t, uint64(taskCount/2), pool.SuccessfulTasks())
}

func TestResultPoolSubpool(t *testing.T) {

	pool := NewResultPool[int](1000)
	subpool := pool.NewSubpool(10)

	var executedCount atomic.Int64

	for i := 0; i < 100; i++ {
		i := i
		subpool.SubmitErr(func() (int, error) {
			executedCount.Add(1)
			return i, nil
		})
	}

	subpool.StopAndWait()

	assert.Equal(t, int64(100), executedCount.Load())
}

func TestResultSubpoolMaxConcurrency(t *testing.T) {
	pool := NewResultPool[int](10)

	assert.PanicsWithError(t, "maxConcurrency must be greater than or equal to 0", func() {
		pool.NewSubpool(-1)
	})

	assert.PanicsWithError(t, "maxConcurrency cannot be greater than the parent pool's maxConcurrency (10)", func() {
		pool.NewSubpool(11)
	})

	subpool := pool.NewSubpool(0)

	assert.Equal(t, 10, subpool.MaxConcurrency())
}

func TestResultPoolTrySubmit(t *testing.T) {
	pool := NewResultPool[int](1, WithQueueSize(1))
	defer pool.StopAndWait()

	completeFirstTask := make(chan struct{})

	// Test successful submission
	task, ok := pool.TrySubmit(func() int {
		completeFirstTask <- struct{}{}
		return 42
	})
	assert.True(t, ok)

	// Test submission to queue
	task2, ok := pool.TrySubmit(func() int {
		return 43
	})
	assert.True(t, ok)

	// Test failed submission when queue is full
	task3, ok := pool.TrySubmit(func() int {
		return 44
	})
	assert.True(t, !ok)

	<-completeFirstTask

	// Test submission to stopped pool
	pool.StopAndWait()
	_, ok = pool.TrySubmit(func() int {
		return 45
	})
	assert.True(t, !ok)

	// Verify tasks completed successfully
	result, err := task.Wait()
	assert.Equal(t, nil, err)
	assert.Equal(t, 42, result)

	result, err = task2.Wait()
	assert.Equal(t, nil, err)
	assert.Equal(t, 43, result)

	result, err = task3.Wait()
	assert.Equal(t, ErrQueueFull, err)
	assert.Equal(t, 0, result)

	// Verify metrics
	assert.Equal(t, uint64(3), pool.SubmittedTasks())
	assert.Equal(t, uint64(2), pool.CompletedTasks())
	assert.Equal(t, uint64(2), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.FailedTasks())
	assert.Equal(t, uint64(1), pool.DroppedTasks())
}

func TestResultPoolTrySubmitErr(t *testing.T) {
	pool := NewResultPool[int](1, WithQueueSize(1))
	defer pool.StopAndWait()

	completeFirstTask := make(chan struct{})
	sampleErr := errors.New("sample error")

	// Test successful submission
	task, ok := pool.TrySubmitErr(func() (int, error) {
		completeFirstTask <- struct{}{}
		return 42, nil
	})
	assert.True(t, ok)

	// Test submission to queue with error
	task2, ok := pool.TrySubmitErr(func() (int, error) {
		return 0, sampleErr
	})
	assert.True(t, ok)

	// Test failed submission when queue is full
	task3, ok := pool.TrySubmitErr(func() (int, error) {
		return 44, nil
	})
	assert.True(t, !ok)

	<-completeFirstTask

	// Test submission to stopped pool
	pool.StopAndWait()
	_, ok = pool.TrySubmitErr(func() (int, error) {
		return 45, nil
	})
	assert.True(t, !ok)

	// Verify tasks completed successfully
	result, err := task.Wait()
	assert.Equal(t, nil, err)
	assert.Equal(t, 42, result)

	result, err = task2.Wait()
	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 0, result)

	result, err = task3.Wait()
	assert.Equal(t, ErrQueueFull, err)
	assert.Equal(t, 0, result)

	// Verify metrics
	assert.Equal(t, uint64(3), pool.SubmittedTasks())
	assert.Equal(t, uint64(2), pool.CompletedTasks())
	assert.Equal(t, uint64(1), pool.SuccessfulTasks())
	assert.Equal(t, uint64(1), pool.FailedTasks())
	assert.Equal(t, uint64(1), pool.DroppedTasks())
}
