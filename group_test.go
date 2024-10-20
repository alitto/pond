package pond

import (
	"errors"
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

func TestTaskGroupDone(t *testing.T) {

	pool := NewResultPool[int](10)

	group := pool.NewGroup()

	for i := 0; i < 5; i++ {
		i := i
		group.SubmitErr(func() (int, error) {
			time.Sleep(1 * time.Millisecond)
			return i, nil
		})
	}

	<-group.Done()

	outputs, err := group.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(outputs))
	assert.Equal(t, 0, outputs[0])
	assert.Equal(t, 1, outputs[1])
	assert.Equal(t, 2, outputs[2])
	assert.Equal(t, 3, outputs[3])
	assert.Equal(t, 4, outputs[4])
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
