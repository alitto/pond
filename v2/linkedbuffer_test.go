package pond

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestLinkedBufferRead(t *testing.T) {
	buf := NewLinkedBuffer[int](10, 10240)

	writeCount := 10000
	writers := 1000
	readers := 1000
	readCount := int64(0)

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

	assertEqual(t, int64(writers*writeCount), buf.Len())

	for i := 0; i < readers; i++ {
		go func() {
			defer readersWg.Done()

			batch := make([]int, 2000)

			for {
				batchSize := buf.Read(batch)

				if batchSize == 0 {
					break
				}

				atomic.AddInt64(&readCount, int64(batchSize))

				// Reset buffer
				batch = batch[:0]
			}
		}()
	}

	readersWg.Wait()

	assertEqual(t, int64(writers*writeCount), atomic.LoadInt64(&readCount))
}
