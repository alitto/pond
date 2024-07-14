package unboundedchanatomic

import (
	"errors"
	"sync/atomic"
)

var ErrIsEmpty = errors.New("linkedbuffer is empty")

type LinkedBuffer[T any] struct {
	// Reader
	readBuffer atomic.Value

	// Writer
	writeBuffer atomic.Value
}

func newLinkedBuffer[T any](initialCapacity int64) *LinkedBuffer[T] {
	initialBuffer := newBuffer[T](initialCapacity)

	buffer := &LinkedBuffer[T]{}

	buffer.readBuffer.Store(initialBuffer)
	buffer.writeBuffer.Store(initialBuffer)

	return buffer
}

func NewLinkedBuffer[T any](initialCapacity int64) *LinkedBuffer[T] {
	return newLinkedBuffer[T](initialCapacity)
}

func (b *LinkedBuffer[T]) Write(values ...T) {
	var writeBuffer *buffer[T]
	// Append elements to the buffer
	for _, value := range values {
		for {
			writeBuffer = b.writeBuffer.Load().(*buffer[T])

			// Write element
			written := writeBuffer.Write(value)

			if written {
				break
			}

			// Increase next buffer capacity
			var newCapacity int64
			if writeBuffer.capacity < 1024 {
				newCapacity = writeBuffer.capacity * 2
			} else {
				newCapacity = writeBuffer.capacity + writeBuffer.capacity/4
			}

			newBuffer := newBuffer[T](newCapacity)
			swapped := writeBuffer.Next.CompareAndSwap(nil, newBuffer)
			if swapped {
				b.writeBuffer.CompareAndSwap(writeBuffer, newBuffer)
			}
		}
	}
}

func (b *LinkedBuffer[T]) Read() (T, error) {
	var readBuffer *buffer[T]
	for {
		readBuffer = b.readBuffer.Load().(*buffer[T])

		// Read element
		value, empty, read := readBuffer.Read()

		if empty {
			// No more elements in the buffer
			return value, ErrIsEmpty
		}

		if read {
			// Move to next buffer
			nextReadBuffer := readBuffer.Next.Load()
			if nextReadBuffer == nil {
				return value, ErrIsEmpty
			}
			b.readBuffer.CompareAndSwap(readBuffer, nextReadBuffer.(*buffer[T]))
			continue
		}

		return value, nil
	}
}
