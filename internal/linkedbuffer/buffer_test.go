package linkedbuffer

import (
	"fmt"
	"runtime"
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

func TestBufferWithPointerToLargeObject(t *testing.T) {
	var m runtime.MemStats

	type Payload struct {
		data []byte
	}

	runtime.ReadMemStats(&m)

	dataSize := 10 * 1024 * 1024
	data := &Payload{
		data: make([]byte, dataSize),
	}

	buffer := newBuffer[*Payload](10)

	runtime.ReadMemStats(&m)
	before := int64(m.Alloc)

	buffer.Write(data)

	runtime.ReadMemStats(&m)
	after := int64(m.Alloc)

	// Verify large object wasn't copied when writing it to the buffer
	assert.Equal(t, true, after-before < int64(dataSize))

	runtime.ReadMemStats(&m)
	before = int64(m.Alloc)

	readData, err := buffer.Read()

	runtime.ReadMemStats(&m)
	after = int64(m.Alloc)

	// Verify large object wasn't copied while reading
	assert.Equal(t, nil, err)
	assert.Equal(t, cap(data.data), cap(readData.data))
	assert.Equal(t, true, after-before < int64(dataSize))

	runtime.ReadMemStats(&m)
	before = int64(m.Alloc)

	// Remove references to the large object
	data.data = make([]byte, 0)
	readData.data = make([]byte, 0)

	// Trigger garbage collection
	runtime.GC()

	runtime.ReadMemStats(&m)
	after = int64(m.Alloc)

	// Verify large object was garbage collected (no internal references to the large object remain)
	assert.Equal(t, true, before-after >= (int64(dataSize)/2))

	// Keep a reference to these variables to ensure they are not discarded by GC
	fmt.Printf("%#v, %#v\n", data.data, readData.data)
}
