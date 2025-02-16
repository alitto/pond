package semaphore_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	syncsema "golang.org/x/sync/semaphore"

	"github.com/alitto/pond/v2/internal/semaphore"
)

func BenchmarkWeighted(b *testing.B) {
	// This is a benchmark for the Weighted semaphore. It is not
	// intended to be a benchmark for the semaphore package as a whole.

	goroutines := 1000000
	semaphoreSize := 100
	maxWeight := 1
	wait := 1 * time.Microsecond

	weights := make([]int, goroutines)

	for i := 0; i < goroutines; i++ {
		weights[i] = rand.Intn(maxWeight) + 1
	}

	b.Run("Channel", func(b *testing.B) {
		sem := make(chan struct{}, semaphoreSize)

		wg := sync.WaitGroup{}
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			weight := weights[i]
			go func() {
				defer wg.Done()
				// Acquire
				for i := 0; i < weight; i++ {
					sem <- struct{}{}
				}
				time.Sleep(wait)
				// Release
				for i := 0; i < weight; i++ {
					<-sem
				}
			}()
		}

		wg.Wait()
	})

	b.Run("Weighted", func(b *testing.B) {
		ctx := context.Background()
		sem := semaphore.NewWeighted(semaphoreSize)

		wg := sync.WaitGroup{}
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			weight := weights[i]
			go func() {
				defer wg.Done()
				sem.Acquire(ctx, weight)
				time.Sleep(wait)
				sem.Release(weight)
			}()
		}

		wg.Wait()
	})

	b.Run("x/sync/semaphore", func(b *testing.B) {
		ctx := context.Background()
		sem := syncsema.NewWeighted(int64(semaphoreSize))

		wg := sync.WaitGroup{}
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			weight := weights[i]
			go func() {
				defer wg.Done()
				sem.Acquire(ctx, int64(weight))
				time.Sleep(wait)
				sem.Release(int64(weight))
			}()
		}

		wg.Wait()
	})

}
