package linkedbuffer

import (
	"math"
	"sync/atomic"
)

// LinkedBuffer implements an unbounded generic buffer that can be written to and read from concurrently.
// It is implemented using a linked list of buffers.
type LinkedBuffer[T any] struct {
	// Reader points to the buffer that is currently being read
	readBuffer *buffer[T]

	// Writer points to the buffer that is currently being written
	writeBuffer *buffer[T]

	maxCapacity int
	writeCount  atomic.Uint64
	readCount   atomic.Uint64
}

func NewLinkedBuffer[T any](initialCapacity, maxCapacity int) *LinkedBuffer[T] {
	initialBuffer := newBuffer[T](initialCapacity)

	buffer := &LinkedBuffer[T]{
		readBuffer:  initialBuffer,
		writeBuffer: initialBuffer,
		maxCapacity: maxCapacity,
	}

	return buffer
}

// Write writes values to the buffer
func (b *LinkedBuffer[T]) Write(value T) {

	// Write elements
	err := b.writeBuffer.Write(value)

	if err == ErrEOF {
		// Increase next buffer capacity
		var newCapacity int
		capacity := b.writeBuffer.Cap()
		if capacity < 1024 {
			newCapacity = capacity * 2
		} else {
			newCapacity = capacity + capacity/2
		}
		if newCapacity > b.maxCapacity {
			newCapacity = b.maxCapacity
		}

		if b.writeBuffer.next == nil {
			b.writeBuffer.next = newBuffer[T](newCapacity)
			b.writeBuffer = b.writeBuffer.next
		}

		// Retry writing
		b.Write(value)
		return
	}

	// Increment written count
	b.writeCount.Add(1)
}

// Read reads values from the buffer and returns the number of elements read
func (b *LinkedBuffer[T]) Read() (value T, err error) {
	// Read element
	value, err = b.readBuffer.Read()

	if err == ErrEOF {
		if b.readBuffer.next == nil {
			// No more elements to read
			return
		}
		// Move to next read buffer
		if b.readBuffer != b.readBuffer.next {
			b.readBuffer = b.readBuffer.next
		}

		// Retry reading
		return b.Read()
	}

	// Increment read count
	b.readCount.Add(1)

	return
}

// WriteCount returns the number of elements written to the buffer since it was created
func (b *LinkedBuffer[T]) WriteCount() uint64 {
	return b.writeCount.Load()
}

// ReadCount returns the number of elements read from the buffer since it was created
func (b *LinkedBuffer[T]) ReadCount() uint64 {
	return b.readCount.Load()
}

// Len returns the number of elements in the buffer that haven't yet been read
func (b *LinkedBuffer[T]) Len() uint64 {
	writeCount := b.writeCount.Load()
	readCount := b.readCount.Load()

	if writeCount < readCount {
		// The writeCount counter wrapped around
		return math.MaxUint64 - readCount + writeCount
	}

	return writeCount - readCount
}
