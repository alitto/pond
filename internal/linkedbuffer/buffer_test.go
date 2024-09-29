package linkedbuffer

import (
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestBuffer(t *testing.T) {

	buffer := NewBuffer[int](10)

	assert.Equal(t, 10, buffer.Cap())

	// Read should return 0 elements
	n, err := buffer.Read(make([]int, 10))
	assert.Equal(t, 0, n)
	assert.Equal(t, nil, err)

	// Write should write 5 elements
	n, err = buffer.Write([]int{1, 2, 3, 4, 5})
	assert.Equal(t, 5, n)
	assert.Equal(t, nil, err)

	// Write should write 5 elements even if 6 are provided
	n, err = buffer.Write([]int{6, 7, 8, 9, 10, 11})
	assert.Equal(t, 5, n)
	assert.Equal(t, nil, err)

	// Read should read 10 elements
	values := make([]int, 10)
	n, err = buffer.Read(values)
	assert.Equal(t, 10, n)
	assert.Equal(t, nil, err)
	for i := 0; i < 10; i++ {
		assert.Equal(t, i+1, values[i])
	}

	// Read should return 0 elements and EOF error
	n, err = buffer.Read(make([]int, 10))
	assert.Equal(t, 0, n)
	assert.Equal(t, ErrEOF, err)
}
