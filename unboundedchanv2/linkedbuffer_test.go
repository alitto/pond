package unboundedchanv2

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func assertEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

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
				buf.Write(workerNum * n)
			}
		}()
	}

	writersWg.Wait()

	assertEqual(t, int64(writers*writeCount), buf.Len())

	for i := 0; i < readers; i++ {
		go func() {
			defer readersWg.Done()

			for {
				if _, err := buf.Read(); err != nil {
					fmt.Printf("Failed to read next element %v\n", err)
					break
				}

				atomic.AddInt64(&readCount, 1)
			}
		}()
	}

	readersWg.Wait()

	assertEqual(t, int64(writers*writeCount), atomic.LoadInt64(&readCount))
}

func TestLinkedBufferReadAll(t *testing.T) {
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
				buf.Write(workerNum * n)
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
				batchSize, err := buf.ReadAll(batch)
				if err != nil {
					fmt.Printf("Failed to read next element %v\n", err)
					break
				}

				atomic.AddInt64(&readCount, batchSize)

				// Reset buffer
				batch = batch[:0]
			}
		}()
	}

	readersWg.Wait()

	assertEqual(t, int64(writers*writeCount), atomic.LoadInt64(&readCount))
}
