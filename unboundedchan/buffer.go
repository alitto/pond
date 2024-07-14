package unboundedchan

import (
	"sync"
	"sync/atomic"
)

type buffer[T any] struct {
	data           []T
	nextWriteIndex atomic.Int64
	nextReadIndex  atomic.Int64
	mutex          sync.Mutex
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

func (b *buffer[T]) Write(value T) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	nextWriteIndex := b.nextWriteIndex.Load()

	if nextWriteIndex >= int64(b.Cap()) {
		// Buffer is full
		return false
	}

	b.data[nextWriteIndex] = value

	b.nextWriteIndex.Add(1)

	return true
}

func (b *buffer[T]) Read() (value T, empty, read bool) {
	readIndex := b.nextReadIndex.Add(1) - 1
	if readIndex >= int64(b.Cap()) {
		// Buffer read completely
		read = true
		return
	}

	if readIndex >= b.nextWriteIndex.Load() {
		// Buffer is empty
		b.nextReadIndex.Add(-1)
		empty = true
		return
	}

	value = b.data[readIndex]
	return
}

func (b *buffer[T]) ReadAll() (value []T, empty, read bool) {
	nextWriteIndex := b.nextWriteIndex.Load()
	pendingCount := nextWriteIndex - b.nextReadIndex.Load()
	newNextReadIndex := b.nextReadIndex.Add(pendingCount)
	readIndex := newNextReadIndex - pendingCount
	if readIndex >= int64(b.Cap()) {
		// Buffer read completely
		read = true
		return
	}

	unreadCount := nextWriteIndex - readIndex
	if unreadCount <= 0 {
		// There are no unread elements in the buffer
		b.nextReadIndex.Add(-pendingCount) // Return unread elements
		empty = true
		return
	}
	if unreadCount < pendingCount {
		b.nextReadIndex.Add(unreadCount - pendingCount) // Return unread elements
	}

	value = b.data[readIndex : readIndex+unreadCount]
	return
}
