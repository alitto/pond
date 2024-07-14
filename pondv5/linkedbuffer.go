package pondv5

import (
	"sync"
	"sync/atomic"
)

type LinkedBuffer[T any] struct {
	// Reader
	readBuffer *buffer[T]

	// Writer
	writeBuffer *buffer[T]

	maxCapacity int
	writeCount  atomic.Int64
	readCount   atomic.Int64
	mutex       sync.RWMutex
}

func newLinkedBuffer[T any](initialCapacity, maxCapacity int) *LinkedBuffer[T] {
	initialBuffer := newBuffer[T](initialCapacity)

	buffer := &LinkedBuffer[T]{
		readBuffer:  initialBuffer,
		writeBuffer: initialBuffer,
		maxCapacity: maxCapacity,
	}

	return buffer
}

func NewLinkedBuffer[T any](initialCapacity, maxCapacity int) *LinkedBuffer[T] {
	return newLinkedBuffer[T](initialCapacity, maxCapacity)
}

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
				b.writeBuffer.next = newBuffer[T](newCapacity)
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
	b.writeCount.Add(int64(length))
}

func (b *LinkedBuffer[T]) Read(values []T) int {

	var readBuffer *buffer[T]

	for {
		b.mutex.RLock()
		readBuffer = b.readBuffer
		b.mutex.RUnlock()

		// Read element
		n, err := readBuffer.Read(values)

		if err == ErrEOF {
			// Move to next buffer
			b.mutex.Lock()
			if readBuffer.next == nil {
				b.mutex.Unlock()
				return n
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			b.mutex.Unlock()
			continue
		}

		if n > 0 {
			// Increment read count
			b.readCount.Add(int64(n))
		}

		return n
	}
}

func (b *LinkedBuffer[T]) WriteCount() int64 {
	return b.writeCount.Load()
}

func (b *LinkedBuffer[T]) ReadCount() int64 {
	return b.readCount.Load()
}

// Len returns the number of elements in the buffer that haven't been read
func (b *LinkedBuffer[T]) Len() int64 {
	return b.writeCount.Load() - b.readCount.Load()
}
