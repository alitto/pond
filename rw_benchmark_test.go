package pond

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alitto/pond/unboundedchan"
	"github.com/alitto/pond/unboundedchanatomic"
)

type ReadWritableChannel[T any] interface {
	Write(values ...T)
	Out() <-chan T
	Close()
}

type readWriteBenchmark[T any] struct {
	name               string
	channelConstructor func() ReadWritableChannel[T]
}

var readWriteBenchmarks = []readWriteBenchmark[int]{
	{
		name: "atomic",
		channelConstructor: func() ReadWritableChannel[int] {
			return unboundedchanatomic.NewUnboundedChan[int](context.Background(), 1024)
		},
	}, {
		name: "mutex",
		channelConstructor: func() ReadWritableChannel[int] {
			return unboundedchan.NewUnboundedChan[int](context.Background(), 1024)
		},
	},
}

func BenchmarkUnboundedChanReadWrite(b *testing.B) {

	writers := 1000
	readers := 1000
	writeCount := 10000

	for _, bench := range readWriteBenchmarks {
		b.Run(bench.name, func(b *testing.B) {

			channel := bench.channelConstructor()
			readCount := int64(0)
			writerWg := sync.WaitGroup{}
			writerWg.Add(writers)
			readerWg := sync.WaitGroup{}
			readerWg.Add(readers)

			// Launch goroutines that submit many elements to the unbounded channel
			for i := 0; i < writers; i++ {
				workerNum := i
				go func() {
					defer writerWg.Done()
					for i := 0; i < writeCount; i++ {
						channel.Write(workerNum + i)
					}
				}()
			}

			// Launch goroutines that read elements from the unbounded channel
			for i := 0; i < readers; i++ {
				go func() {
					defer readerWg.Done()
					for range channel.Out() {
						atomic.AddInt64(&readCount, 1)
					}
				}()
			}

			// Wait for both readers and writers
			writerWg.Wait()
			channel.Close()
			readerWg.Wait()
		})
	}
}
