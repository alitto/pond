package pond

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDispatcher(t *testing.T) {

	ctx := context.Background()

	writers := 1000
	writeCount := 10000

	writerWg := sync.WaitGroup{}
	writerWg.Add(writers)
	readerWg := sync.WaitGroup{}
	readerWg.Add(writers * writeCount)
	readCount := int64(0)

	receiveFunc := func(elems []int) {
		for range elems {
			atomic.AddInt64(&readCount, 1)
			readerWg.Done()
		}
	}

	dispatcher := newDispatcher(ctx, receiveFunc, 2048)

	// Launch goroutines that submit many elements to the unbounded channel
	for i := 0; i < writers; i++ {
		workerNum := i
		go func() {
			for i := 0; i < writeCount; i++ {
				dispatcher.Write(workerNum*10000 + i)
			}
			writerWg.Done()
		}()
	}

	// Wait for both readers and writers
	writerWg.Wait()
	dispatcher.Close()
	readerWg.Wait()

	fmt.Printf("Read %d elements", atomic.LoadInt64(&readCount))

	assertEqual(t, int64(writers*writeCount), atomic.LoadInt64(&readCount))
}
