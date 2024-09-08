package pond

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcher(t *testing.T) {

	ctx := context.Background()

	writers := 100
	writeCount := 1000

	writerWg := sync.WaitGroup{}
	writerWg.Add(writers)
	readerWg := sync.WaitGroup{}
	readerWg.Add(writers * writeCount)
	var receivedCount atomic.Uint64

	receiveFunc := func(elems []int) {
		for range elems {
			receivedCount.Add(1)
			readerWg.Done()
		}
	}

	dispatcher := newDispatcher(ctx, receiveFunc, 1024)

	// Assert counters
	assertEqual(t, uint64(0), dispatcher.Len())
	assertEqual(t, uint64(0), dispatcher.WriteCount())
	assertEqual(t, uint64(0), dispatcher.ReadCount())

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

	// Assert counters
	assertEqual(t, uint64(writers*writeCount), receivedCount.Load())
	assertEqual(t, uint64(0), dispatcher.Len())
	assertEqual(t, uint64(writers*writeCount), dispatcher.ReadCount())
	assertEqual(t, uint64(writers*writeCount), dispatcher.WriteCount())
}

func TestDispatcherWithContextCanceled(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	receivedCount := atomic.Uint64{}
	receiveFunc := func(elems []int) {
		for range elems {
			receivedCount.Add(1)
		}
	}

	dispatcher := newDispatcher(ctx, receiveFunc, 1024)

	// Assert counters
	assertEqual(t, uint64(0), dispatcher.Len())
	assertEqual(t, uint64(0), dispatcher.WriteCount())
	assertEqual(t, uint64(0), dispatcher.ReadCount())

	// Cancel the context
	cancel()
	// Write to the dispatcher
	dispatcher.Write(1)
	time.Sleep(5 * time.Millisecond)

	// Assert counters
	assertEqual(t, uint64(1), dispatcher.Len())
	assertEqual(t, uint64(1), dispatcher.WriteCount())
	assertEqual(t, uint64(0), dispatcher.ReadCount())
}

func TestDispatcherWithContextCanceledAfterWrite(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	receivedCount := atomic.Uint64{}
	receiveFunc := func(elems []int) {
		for range elems {
			receivedCount.Add(1)
		}
	}

	dispatcher := newDispatcher(ctx, receiveFunc, 1024)

	// Assert counters
	assertEqual(t, uint64(0), dispatcher.Len())
	assertEqual(t, uint64(0), dispatcher.WriteCount())
	assertEqual(t, uint64(0), dispatcher.ReadCount())

	// Cancel the context
	dispatcher.Write(1)
	time.Sleep(5 * time.Millisecond) // Wait for the dispatcher to process the element
	cancel()
	dispatcher.Write(1)
	time.Sleep(5 * time.Millisecond) // Wait for the dispatcher to process the element

	// Assert counters
	assertEqual(t, uint64(1), dispatcher.Len())
	assertEqual(t, uint64(2), dispatcher.WriteCount())
	assertEqual(t, uint64(1), dispatcher.ReadCount())
}

func TestDispatcherWriteAfterClose(t *testing.T) {

	ctx := context.Background()

	receivedCount := atomic.Uint64{}
	receiveFunc := func(elems []int) {
		for range elems {
			receivedCount.Add(1)
		}
	}

	dispatcher := newDispatcher(ctx, receiveFunc, 1024)

	// Close the dispatcher
	dispatcher.Close()

	// Write to the dispatcher
	err := dispatcher.Write(1)
	time.Sleep(5 * time.Millisecond)

	// Assert counters
	assertEqual(t, ErrDispatcherClosed, err)
	assertEqual(t, uint64(0), dispatcher.Len())
	assertEqual(t, uint64(0), dispatcher.WriteCount())
	assertEqual(t, uint64(0), dispatcher.ReadCount())
}
