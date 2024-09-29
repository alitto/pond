package pond

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestTaskGroupGet(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()

	for i := 0; i < 5; i++ {
		group.Add(func() (int, error) {
			return i, nil
		})
	}

	outputs, err := group.Get()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(outputs))
	assert.Equal(t, 0, outputs[0])
	assert.Equal(t, 1, outputs[1])
	assert.Equal(t, 2, outputs[2])
	assert.Equal(t, 3, outputs[3])
	assert.Equal(t, 4, outputs[4])
}

func TestTaskGroupGetWithError(t *testing.T) {

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

	outputs, err := group.Get()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 0, len(outputs))
}

func TestTaskGroupGetWithMultipleErrors(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()

	sampleErr := errors.New("sample error")

	for i := 0; i < 5; i++ {
		group.Add(func() (int, error) {
			if i%2 == 0 {
				return 0, sampleErr
			}
			return i, nil
		})
	}

	outputs, err := group.Get()

	assert.Equal(t, 0, len(outputs))
	assert.Equal(t, sampleErr, err)
}

func TestTaskGroupGetAll(t *testing.T) {

	pool := NewTypedPool[int](10)

	group := pool.Group()
	groupAll := pool.Group()

	for i := 0; i < 5; i++ {
		task := func() (int, error) {
			if i%2 == 0 {
				return 0, fmt.Errorf("error %d", i)
			}
			return i, nil
		}
		group.Add(task)
		groupAll.Add(task)
	}

	results, err := group.Get()

	assert.True(t, err != nil)
	assert.Equal(t, 0, len(results))
	assert.True(t, strings.Contains(err.Error(), "error"))

	resultsAll, err := groupAll.GetAll()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, len(resultsAll))
	assert.Equal(t, 0, resultsAll[0].Output)
	assert.Equal(t, 1, resultsAll[1].Output)
	assert.Equal(t, 0, resultsAll[2].Output)
	assert.Equal(t, 3, resultsAll[3].Output)
	assert.Equal(t, 0, resultsAll[4].Output)
	assert.Equal(t, "error 0", resultsAll[0].Err.Error())
	assert.Equal(t, "error 2", resultsAll[2].Err.Error())
	assert.Equal(t, "error 4", resultsAll[4].Err.Error())
}
