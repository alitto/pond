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

func TestGroupSubmitWithFluentSyntax(t *testing.T) {

	results, err := WithResult[string]().
		Group(func() string {
			return "hello"
		}, func() string {
			return "world"
		}).
		Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "hello", results[0])
	assert.Equal(t, "world", results[1])
}
