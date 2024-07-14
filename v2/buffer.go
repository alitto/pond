package pond

import (
	"errors"
	"sync/atomic"
)

var ErrEOF = errors.New("EOF")

type buffer[T any] struct {
	data           []T
	nextWriteIndex atomic.Int64
	nextReadIndex  atomic.Int64
	next           *buffer[T]
}

func newBuffer[T any](capacity int) *buffer[T] {
	return &buffer[T]{
		data: make([]T, capacity),
	}
}

func (b *buffer[T]) Cap() int {
	return cap(b.data)
}

func (b *buffer[T]) Write(values []T) (n int, err error) {
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

func (b *buffer[T]) Read(values []T) (n int, err error) {
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
