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

	pool.EnableDebug()

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
	pool.EnableDebug()

	pool.StopAndWait()

	err := pool.NewGroup().Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestTaskGroupWithContextCanceled(t *testing.T) {

	pool := NewPool(100)
	pool.EnableDebug()

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

	results, err := group.Submit().Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(results))

	results, err = group.SubmitErr().Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(results))
}

func TestTaskGroupCanceledShouldSkipRemainingTasks(t *testing.T) {

	pool := NewPool(1)
	pool.EnableDebug()

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

	pool.EnableDebug()

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
	pool.EnableDebug()

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
	pool.EnableDebug()

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
