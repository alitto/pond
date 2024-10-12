package pond

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestGenericPoolSubmitAndWait(t *testing.T) {

	pool := WithResult[int]().NewPool(1000)
	defer pool.StopAndWait()

	task := pool.Submit(func() int {
		return 5
	})

	output, err := task.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, output)
}

func TestGenericPoolSubmitTaskWithPanic(t *testing.T) {

	pool := WithResult[int]().NewPool(1000)

	task := pool.Submit(func() int {
		panic("dummy panic")
	})

	output, err := task.Wait()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.Equal(t, "task panicked: dummy panic", err.Error())
	assert.Equal(t, 0, output)
}

func TestGenericPoolMetrics(t *testing.T) {

	pool := WithResult[int]().NewPool(1000)

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
		n := i
		pool.SubmitErr(func() (int, error) {
			executedCount.Add(1)
			if n%2 == 0 {
				return n, nil
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
