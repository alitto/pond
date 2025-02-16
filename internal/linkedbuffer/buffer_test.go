package linkedbuffer

import (
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestBuffer(t *testing.T) {

	buffer := newBuffer[int](10)

	assert.Equal(t, 10, buffer.Cap())

	// Read should return 0 elements
	value, err := buffer.Read()
	assert.Equal(t, 0, value)
	assert.Equal(t, ErrEOF, err)

	// Write should write elements
	err = buffer.Write(1)
	assert.Equal(t, nil, err)

	err = buffer.Write(2)
	assert.Equal(t, nil, err)

	// Read should read elements
	value, err = buffer.Read()
	assert.Equal(t, 1, value)
	assert.Equal(t, nil, err)

	value, err = buffer.Read()
	assert.Equal(t, 2, value)
	assert.Equal(t, nil, err)

	// Read should return 0 elements and EOF error
	value, err = buffer.Read()
	assert.Equal(t, 0, value)
	assert.Equal(t, ErrEOF, err)
}
