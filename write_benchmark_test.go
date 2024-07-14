package pond

import (
	"context"
	"sync"
	"testing"

	"github.com/alitto/pond/unboundedchan"
	"github.com/alitto/pond/unboundedchanatomic"
)

type WritableChannel[T any] interface {
	Write(values ...T)
	Close()
}

type writeBenchmark[T any] struct {
	name               string
	channelConstructor func() WritableChannel[T]
}

var benchmarks = []writeBenchmark[int]{
	{
		name: "atomic",
		channelConstructor: func() WritableChannel[int] {
			return unboundedchanatomic.NewUnboundedChan[int](context.Background(), 1024)
		},
	},
	{
		name: "mutex",
		channelConstructor: func() WritableChannel[int] {
			return unboundedchan.NewUnboundedChan[int](context.Background(), 1024)
		},
	},
}

func BenchmarkUnboundedChanWrite(b *testing.B) {

	writers := 100
	writeCount := 10000

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {

			channel := bench.channelConstructor()
			writerWg := sync.WaitGroup{}
			writerWg.Add(writers)

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

			// Wait for both readers and writers
			writerWg.Wait()
			channel.Close()
		})
	}
}
