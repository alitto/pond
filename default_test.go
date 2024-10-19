package pond

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestSubmit(t *testing.T) {

	done := make(chan int, 1)
	task := Submit(func() {
		done <- 10
	})

	err := task.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 10, <-done)
}

func TestSubmitWithError(t *testing.T) {

	task := SubmitErr(func() error {
		return errors.New("sample error")
	})

	err := task.Wait()

	assert.Equal(t, "sample error", err.Error())
}

func TestSubmitWithPanic(t *testing.T) {

	task := Submit(func() {
		panic("dummy panic")
	})

	err := task.Wait()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.Equal(t, "task panicked: dummy panic", err.Error())
}

func TestNewGroup(t *testing.T) {

	group := NewGroup()

	count := 10
	var done atomic.Int32

	for i := 0; i < count; i++ {
		group.SubmitErr(func() error {
			done.Add(1)
			return nil
		})
	}

	err := group.Wait()
	assert.Equal(t, nil, err)
	assert.Equal(t, count, int(done.Load()))
}

func TestNewSubpool(t *testing.T) {

	pool := NewSubpool(10)

	count := 10
	var done atomic.Int32

	for i := 0; i < count; i++ {
		pool.SubmitErr(func() error {
			done.Add(1)
			return nil
		})
	}

	pool.StopAndWait()

	assert.Equal(t, count, int(done.Load()))
}
