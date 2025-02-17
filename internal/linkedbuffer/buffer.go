package linkedbuffer

import (
	"errors"
)

var ErrEOF = errors.New("EOF")

// buffer implements a generic buffer that can store any type of data.
// It is not thread-safe and should be used with a mutex.
// It is used by LinkedBuffer to store data and is not intended to be used directly.
type buffer[T any] struct {
	data           []T
	nextWriteIndex int
	nextReadIndex  int
	next           *buffer[T]
}

func newBuffer[T any](capacity int) *buffer[T] {
	return &buffer[T]{
		data: make([]T, capacity),
	}
}

// Cap returns the capacity of the buffer.
func (b *buffer[T]) Cap() int {
	return cap(b.data)
}

// Len returns the number of elements in the buffer.
func (b *buffer[T]) Len() int {
	return b.nextWriteIndex - b.nextReadIndex
}

// Write writes a value to the buffer.
// If the buffer is full, it returns an EOF error.
func (b *buffer[T]) Write(value T) error {
	if b.nextWriteIndex >= b.Cap() {
		// Buffer is full
		return ErrEOF
	}

	b.data[b.nextWriteIndex] = value
	b.nextWriteIndex++
	return nil
}

// Read reads a value from the buffer.
// If the buffer is empty, it returns an EOF error.
// If the buffer has been read completely, it returns an EOF error.
func (b *buffer[T]) Read() (value T, err error) {
	if b.nextReadIndex >= b.Cap() || (b.next == nil && b.nextReadIndex >= b.nextWriteIndex) {
		// Buffer read completely, return EOF error
		err = ErrEOF
		return
	}

	value = b.data[b.nextReadIndex]
	b.nextReadIndex++
	return
}
