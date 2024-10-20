package linkedbuffer

import (
	"errors"
	"sync/atomic"
)

var ErrEOF = errors.New("EOF")

// Buffer implements a generic Buffer that can store any type of data.
// It is not thread-safe and should be used with a mutex.
// It is used by LinkedBuffer to store data and is not intended to be used directly.
type Buffer[T any] struct {
	data           []T
	nextWriteIndex atomic.Int64
	nextReadIndex  atomic.Int64
	next           *Buffer[T]
}

func NewBuffer[T any](capacity int) *Buffer[T] {
	return &Buffer[T]{
		data: make([]T, capacity),
	}
}

// Cap returns the capacity of the buffer.
func (b *Buffer[T]) Cap() int {
	return cap(b.data)
}

// Write writes values to the buffer.
func (b *Buffer[T]) Write(values []T) (n int, err error) {
	n = len(values)
	writeIndex := b.nextWriteIndex.Load()

	if writeIndex >= int64(b.Cap()) {
		// Buffer is full
		err = ErrEOF
		n = 0
		return
	}

	if writeIndex+int64(n) > int64(b.Cap()) {
		n = b.Cap() - int(writeIndex)
	}

	copy(b.data[writeIndex:int(writeIndex)+n], values[0:n])

	b.nextWriteIndex.Add(int64(n))
	return
}

// Read reads values from the buffer and returns the number of elements read.
// If the buffer is empty, it returns 0 elements.
// If the buffer has been read completely, it returns an EOF error.
func (b *Buffer[T]) Read(values []T) (n int, err error) {
	nextWriteIndex := b.nextWriteIndex.Load()
	n = cap(values)

	readIndex := b.nextReadIndex.Add(int64(n)) - int64(n)
	if readIndex >= int64(b.Cap()) {
		// Buffer read completely, return EOF error
		err = ErrEOF
		n = 0
		return
	}

	unreadCount := nextWriteIndex - readIndex
	if unreadCount <= 0 {
		// There are no unread elements in the buffer
		b.nextReadIndex.Add(-int64(n)) // Return unread elements
		n = 0
		return
	}
	if unreadCount < int64(n) {
		b.nextReadIndex.Add(unreadCount - int64(n)) // Return unread elements
		n = int(unreadCount)
	}

	copy(values[0:n], b.data[readIndex:int(readIndex)+n])
	return
}
