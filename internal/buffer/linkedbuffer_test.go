package buffer

import (
	"math"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestLinkedBuffer(t *testing.T) {
	buf := NewLinkedBuffer[int](2, 2)

	assert.Equal(t, uint64(0), buf.Len())
	assert.Equal(t, uint64(0), buf.WriteCount())
	assert.Equal(t, uint64(0), buf.ReadCount())

	buf.Write(1)
	buf.Write(2)

	assert.Equal(t, uint64(2), buf.Len())
	assert.Equal(t, uint64(2), buf.WriteCount())
	assert.Equal(t, uint64(0), buf.ReadCount())

	value, err := buf.Read()

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, value)
	assert.Equal(t, uint64(1), buf.ReadCount())

	value, err = buf.Read()

	assert.Equal(t, 2, value)
	assert.Equal(t, uint64(2), buf.ReadCount())

	// Test EOF
	value, err = buf.Read()

	assert.Equal(t, 0, value)
	assert.Equal(t, ErrEOF, err)
	assert.Equal(t, uint64(2), buf.ReadCount())

	buf.Write(3)

	assert.Equal(t, uint64(1), buf.Len())
	assert.Equal(t, uint64(3), buf.WriteCount())
	assert.Equal(t, uint64(2), buf.ReadCount())
}

func TestLinkedBufferLen(t *testing.T) {
	buf := NewLinkedBuffer[int](10, 1024)

	assert.Equal(t, uint64(0), buf.Len())

	buf.Write(1)
	buf.Write(2)

	assert.Equal(t, uint64(2), buf.Len())

	buf.Write(3)

	assert.Equal(t, uint64(3), buf.Len())

	// Test wrap around
	buf.writeCount.Store(0)
	buf.writeCount.Add(math.MaxUint64)
	buf.readCount.Add(math.MaxUint64 - 3)
	assert.Equal(t, uint64(3), buf.Len())
}

func TestLinkedBufferWithReusedBuffer(t *testing.T) {

	buf := NewLinkedBuffer[int](2, 1)

	buf.Write(1)
	buf.Write(2)

	value, err := buf.Read()

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, value)

	value, err = buf.Read()

	assert.Equal(t, 2, value)

	buf.Write(3)
	buf.Write(4)

	value, err = buf.Read()

	assert.Equal(t, 3, value)

	value, err = buf.Read()

	assert.Equal(t, 4, value)
}
