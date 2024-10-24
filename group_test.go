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

	group := NewResultPool[int](10).
		NewGroup()

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
	assert.Equal(t, 0, len(outputs))
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
	assert.Equal(t, 0, len(outputs))
}

func TestResultTaskGroupWaitWithMultipleErrors(t *testing.T) {

	pool := NewResultPool[int](10)

	group := pool.NewGroup()

	sampleErr := errors.New("sample error")

	for i := 0; i < 5; i++ {
		i := i
		group.SubmitErr(func() (int, error) {
			if i%2 == 0 {
				return 0, sampleErr
			}
			return i, nil
		})
	}

	outputs, err := group.Wait()

	assert.Equal(t, 0, len(outputs))
	assert.Equal(t, sampleErr, err)
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

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := group.SubmitErr(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}).Wait()

	assert.Equal(t, context.Canceled, err)
}

func TestTaskGroupWithNoTasks(t *testing.T) {

	group := NewResultPool[int](10).
		NewGroup()

	assert.PanicsWithError(t, "no tasks provided", func() {
		group.Submit()
	})
	assert.PanicsWithError(t, "no tasks provided", func() {
		group.SubmitErr()
	})
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
