package unboundedchanatomic

import (
	"runtime"
	"sync/atomic"
)

var MAX_PROCS = int64(runtime.GOMAXPROCS(0))

type buffer[T any] struct {
	capacity       int64
	data           []T
	nextWriteIndex atomic.Int64
	committedIndex atomic.Int64
	nextReadIndex  atomic.Int64
	Next           atomic.Value
}

func newBuffer[T any](capacity int64) *buffer[T] {
	buf := &buffer[T]{
		capacity: capacity,
		data:     make([]T, capacity),
	}

	return buf
}

func (b *buffer[T]) Write(value T) bool {
	writeIndex := b.nextWriteIndex.Add(1) - 1

	if writeIndex >= b.capacity {
		// Buffer is full
		return false
	}

	b.data[writeIndex] = value

	b.committedIndex.Add(1)

	return true
}

func (b *buffer[T]) Read() (value T, empty, read bool) {
	readIndex := b.nextReadIndex.Add(1) - 1

	if readIndex >= b.capacity {
		// Buffer read completely
		read = true
		return
	}

	committedIndex := b.committedIndex.Load()

	if readIndex >= committedIndex {
		// No elements in the buffer
		b.nextReadIndex.Add(-1) // Return element
		empty = true
		return
	}

	if committedIndex < b.capacity && b.nextWriteIndex.Load() > committedIndex && readIndex >= committedIndex-MAX_PROCS {
		// It is not safe to read the next element
		b.nextReadIndex.Add(-1) // Return element
		empty = true
		return
	}

	value = b.data[readIndex]
	return
}
