package unboundedchan

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnboundedChan(t *testing.T) {

	ctx := context.Background()

	unboundedChan := NewUnboundedChan[int](ctx, 100)

	writers := 10
	writeCount := 1000000
	readers := 10000

	writerWg := sync.WaitGroup{}
	writerWg.Add(writers)
	readerWg := sync.WaitGroup{}
	readerWg.Add(readers)
	readCount := int64(0)

	// Launch goroutines that submit many elements to the unbounded channel
	for i := 0; i < writers; i++ {
		workerNum := i
		go func() {
			for i := 0; i < writeCount; i++ {
				unboundedChan.Write(workerNum*10000 + i)
			}
			writerWg.Done()
		}()
	}

	// Launch a goroutine that reads 10k elements from the unbounded channel
	for i := 0; i < readers; i++ {
		go func() {
			for range unboundedChan.Out() {
				time.Sleep(1 * time.Millisecond)
				atomic.AddInt64(&readCount, 1)
			}
			readerWg.Done()
		}()
	}

	// Wait for both readers and writers
	writerWg.Wait()
	unboundedChan.Close()
	readerWg.Wait()

	fmt.Printf("Read %d elements", atomic.LoadInt64(&readCount))

	assertEqual(t, int64(writers*writeCount), atomic.LoadInt64(&readCount))
}
