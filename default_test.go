package pond

import (
	"errors"
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

func TestSubmitWithPanic(t *testing.T) {

	task := Submit(func() {
		panic("dummy panic")
	})

	err := task.Wait()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.Equal(t, "task panicked: dummy panic", err.Error())
}
