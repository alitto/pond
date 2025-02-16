package linkedbuffer

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestLinkedBuffer(t *testing.T) {
	buf := NewLinkedBuffer[int](10, 1024)

	assert.Equal(t, uint64(0), buf.Len())
	assert.Equal(t, uint64(0), buf.WriteCount())
	assert.Equal(t, uint64(0), buf.ReadCount())

	writeCount := 1000
	writers := 100
	readers := 100
	var readCount atomic.Uint64

	writersWg := sync.WaitGroup{}
	readersWg := sync.WaitGroup{}
	writersWg.Add(writers)
	readersWg.Add(readers)

	// Launch writers
	for i := 0; i < writers; i++ {
		workerNum := i
		go func() {
			defer writersWg.Done()

			for n := 0; n < writeCount; n++ {
				buf.Write([]int{workerNum * n})
			}
		}()
	}

	writersWg.Wait()

	assert.Equal(t, uint64(writers*writeCount), buf.Len())

	for i := 0; i < readers; i++ {
		go func() {
			defer readersWg.Done()

			batch := make([]int, 2000)

			for {
				batchSize := buf.Read(batch)

				if batchSize == 0 {
					break
				}

				readCount.Add(uint64(batchSize))

				// Reset buffer
				batch = batch[:0]
			}
		}()
	}

	readersWg.Wait()

	assert.Equal(t, uint64(writers*writeCount), readCount.Load())
}

func TestLinkedBufferLen(t *testing.T) {
	buf := NewLinkedBuffer[int](10, 1024)

	assert.Equal(t, uint64(0), buf.Len())

	buf.Write([]int{1, 2, 3, 4, 5})

	assert.Equal(t, uint64(5), buf.Len())

	buf.Write([]int{6, 7, 8, 9, 10})

	assert.Equal(t, uint64(10), buf.Len())

	buf.Read(make([]int, 5))

	assert.Equal(t, uint64(5), buf.Len())

	buf.Read(make([]int, 5))

	assert.Equal(t, uint64(0), buf.Len())

	// Test wrap around
	buf.writeCount.Add(math.MaxUint64)
	buf.readCount.Add(math.MaxUint64 - 3)
	assert.Equal(t, uint64(3), buf.Len())
}

func TestLinkedBufferWithReusedBuffer(t *testing.T) {

	buf := NewLinkedBuffer[int](2, 1)

	values := make([]int, 1)

	buf.Write([]int{1})
	buf.Write([]int{2})

	n := buf.Read(values)

	assert.Equal(t, 1, n)
	assert.Equal(t, 1, values[0])

	assert.Equal(t, 1, len(values))
	assert.Equal(t, 1, cap(values))

	n = buf.Read(values)

	assert.Equal(t, 1, n)
	assert.Equal(t, 1, len(values))
	assert.Equal(t, 2, values[0])

	buf.Write([]int{3})
	buf.Write([]int{4})

	n = buf.Read(values)

	assert.Equal(t, 1, n)
	assert.Equal(t, 1, len(values))
	assert.Equal(t, 3, values[0])

	n = buf.Read(values)

	assert.Equal(t, 1, n)
	assert.Equal(t, 1, len(values))
	assert.Equal(t, 4, values[0])
}
