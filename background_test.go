package pond

import (
	"errors"
	"testing"
)

func TestSubmit(t *testing.T) {

	task := Submit[int](func() int {
		return 10
	})

	out, err := task.Get()

	assertEqual(t, nil, err)
	assertEqual(t, 10, out)
}

func TestSubmitWithPanic(t *testing.T) {

	task := Submit[int](func() int {
		panic("dummy panic")
	})

	out, err := task.Get()

	assertEqual(t, true, errors.Is(err, ErrPanic))
	assertEqual(t, "task panicked: dummy panic", err.Error())
	assertEqual(t, 0, out)
}
