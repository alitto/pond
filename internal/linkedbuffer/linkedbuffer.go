package linkedbuffer

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// LinkedBuffer implements an unbounded generic buffer that can be written to and read from concurrently.
// It is implemented using a linked list of buffers.
type LinkedBuffer[T any] struct {
	// Reader points to the buffer that is currently being read
	readBuffer *Buffer[T]

	// Writer points to the buffer that is currently being written
	writeBuffer *Buffer[T]

	maxCapacity int
	writeCount  atomic.Uint64
	readCount   atomic.Uint64
	mutex       sync.Mutex
	Debug       bool
}

func NewLinkedBuffer[T any](initialCapacity, maxCapacity int) *LinkedBuffer[T] {
	initialBuffer := NewBuffer[T](initialCapacity)

	buffer := &LinkedBuffer[T]{
		readBuffer:  initialBuffer,
		writeBuffer: initialBuffer,
		maxCapacity: maxCapacity,
	}

	return buffer
}

// Write writes values to the buffer
func (b *LinkedBuffer[T]) Write(values []T) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	length := len(values)
	nextIndex := 0

	// Append elements to the buffer
	for {
		// Write elements
		n, err := b.writeBuffer.Write(values[nextIndex:])

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
				b.writeBuffer.next = NewBuffer[T](newCapacity)
				b.writeBuffer = b.writeBuffer.next
			}
			continue
		}

		nextIndex += n

		if nextIndex >= length {
			break
		}
	}

	// Increment written count
	b.writeCount.Add(uint64(length))
}

// Read reads values from the buffer and returns the number of elements read
func (b *LinkedBuffer[T]) Read(values []T) int {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	var readBuffer *Buffer[T]

	for {
		readBuffer = b.readBuffer

		// Read element
		n, err := readBuffer.Read(values)

		if b.Debug {
			fmt.Printf("read %d elements: %v\n", n, values)
		}

		if err == ErrEOF {
			// Move to next buffer
			if readBuffer.next == nil {
				return n
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			continue
		}

		if n > 0 {
			// Increment read count
			b.readCount.Add(uint64(n))
		}

		return n
	}
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
		return 0 // Make sure we don't return a negative value
	}

	return writeCount - readCount
}
