package unboundedchan

import (
	"errors"
	"sync"
)

var ErrIsEmpty = errors.New("linkedbuffer is empty")

type LinkedBuffer[T any] struct {
	// Reader
	readBuffer *buffer[T]

	// Writer
	writeBuffer *buffer[T]

	mutex sync.RWMutex
}

func newLinkedBuffer[T any](initialCapacity int) *LinkedBuffer[T] {
	initialBuffer := newBuffer[T](initialCapacity)

	buffer := &LinkedBuffer[T]{
		readBuffer:  initialBuffer,
		writeBuffer: initialBuffer,
	}

	return buffer
}

func NewLinkedBuffer[T any](initialCapacity int) *LinkedBuffer[T] {
	return newLinkedBuffer[T](initialCapacity)
}

func (b *LinkedBuffer[T]) Write(values ...T) {
	var writeBuffer *buffer[T]
	// Append elements to the buffer
	for _, value := range values {
		for {
			b.mutex.RLock()
			writeBuffer = b.writeBuffer
			b.mutex.RUnlock()

			// Write element
			written := writeBuffer.Write(value)

			if written {
				break
			}

			// Increase next buffer capacity
			var newCapacity int
			capacity := writeBuffer.Cap()
			if capacity < 1024 {
				newCapacity = capacity * 2
			} else {
				newCapacity = capacity + capacity/2
			}

			b.mutex.Lock()
			if writeBuffer.next == nil {
				writeBuffer.next = newBuffer[T](newCapacity)
				b.writeBuffer = writeBuffer.next
			}
			b.mutex.Unlock()
		}
	}
}

func (b *LinkedBuffer[T]) Read() (T, error) {
	var readBuffer *buffer[T]
	for {
		b.mutex.RLock()
		readBuffer = b.readBuffer
		b.mutex.RUnlock()

		// Read element
		value, empty, read := readBuffer.Read()

		if empty {
			// No more elements in the buffer
			return value, ErrIsEmpty
		}

		if read {
			// Move to next buffer
			b.mutex.Lock()
			if b.readBuffer.next == nil {
				b.mutex.Unlock()
				return value, ErrIsEmpty
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			b.mutex.Unlock()
			continue
		}

		return value, nil
	}
}

func (b *LinkedBuffer[T]) ReadAll() ([]T, error) {
	var readBuffer *buffer[T]
	for {
		b.mutex.RLock()
		readBuffer = b.readBuffer
		b.mutex.RUnlock()

		// Read element
		value, empty, read := readBuffer.ReadAll()

		if empty {
			// No more elements in the buffer
			return value, ErrIsEmpty
		}

		if read {
			// Move to next buffer
			b.mutex.Lock()
			if b.readBuffer.next == nil {
				b.mutex.Unlock()
				return value, ErrIsEmpty
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			b.mutex.Unlock()
			continue
		}

		return value, nil
	}
}
