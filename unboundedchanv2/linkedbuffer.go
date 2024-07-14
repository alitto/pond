package unboundedchanv2

import (
	"errors"
	"sync"
	"sync/atomic"
)

var ErrIsEmpty = errors.New("linkedbuffer is empty")

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
			if newCapacity > b.maxCapacity {
				newCapacity = b.maxCapacity
			}

			b.mutex.Lock()
			if writeBuffer.next == nil {
				writeBuffer.next = newBuffer[T](newCapacity)
				b.writeBuffer = writeBuffer.next
			}
			b.mutex.Unlock()
		}
		// Increment written count
		b.writeCount.Add(1)
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
			if readBuffer.next == nil {
				b.mutex.Unlock()
				return value, ErrIsEmpty
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			b.mutex.Unlock()
			continue
		}

		// Increment read count
		b.readCount.Add(1)

		return value, nil
	}
}

func (b *LinkedBuffer[T]) ReadAll(values []T) (int64, error) {

	var readBuffer *buffer[T]

	for {
		b.mutex.RLock()
		readBuffer = b.readBuffer
		b.mutex.RUnlock()

		// Read element
		readCount, read := readBuffer.ReadAll(values)

		if read {
			// Move to next buffer
			b.mutex.Lock()
			if readBuffer.next == nil {
				b.mutex.Unlock()
				return readCount, ErrIsEmpty
			}
			if b.readBuffer != readBuffer.next {
				b.readBuffer = readBuffer.next
			}
			b.mutex.Unlock()
			continue
		}

		if readCount == 0 {
			// No more elements in the buffer
			return readCount, ErrIsEmpty
		}

		// Increment read count
		b.readCount.Add(readCount)

		return readCount, nil
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
