package pond

import (
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestSubmitTyped(t *testing.T) {

	task := TypedSubmit[int](func() int {
		return 10
	})

	output, err := task.Get()

	assert.Equal(t, nil, err)
	assert.Equal(t, 10, output)
}

func TestSubmitTypedWithPanic(t *testing.T) {

	task := TypedSubmit[int](func() int {
		panic("dummy panic")
	})

	output, err := task.Get()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.Equal(t, "task panicked: dummy panic", err.Error())
	assert.Equal(t, 0, output)
}
