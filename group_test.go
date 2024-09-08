package pond

import (
	"errors"
	"testing"
)

func TestTaskGroupWait(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()

	for i := 0; i < 5; i++ {
		group.Add(func() (int, error) {
			return i, nil
		})
	}

	outputs, err := group.Submit().Get()

	assertEqual(t, nil, err)
	assertEqual(t, true, outputs != nil)
}

func TestTaskGroupWaitAll(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()

	for i := 0; i < 5; i++ {
		group.Add(func() (int, error) {
			return i, nil
		})
	}

	outputs, err := group.Submit().Get()

	assertEqual(t, nil, err)
	assertEqual(t, 5, len(outputs))
	assertEqual(t, 0, outputs[0])
	assertEqual(t, 1, outputs[1])
	assertEqual(t, 2, outputs[2])
	assertEqual(t, 3, outputs[3])
	assertEqual(t, 4, outputs[4])
}

func TestTaskGroupWithError(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()
	sampleErr := errors.New("sample error")

	for i := 0; i < 5; i++ {
		group.Add(func() (int, error) {
			if i == 3 {
				return 0, sampleErr
			}
			return i, nil
		})
	}

	outputs, err := group.Submit().Get()

	assertEqual(t, sampleErr, err)
	assertEqual(t, 5, len(outputs))
	assertEqual(t, 0, outputs[0])
	assertEqual(t, 1, outputs[1])
	assertEqual(t, 2, outputs[2])
	assertEqual(t, 0, outputs[3])
	assertEqual(t, 0, outputs[4])
}
