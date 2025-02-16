package semaphore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestWeighted(t *testing.T) {
	ctx := context.Background()
	sem := NewWeighted(10)

	// Acquire 5
	err := sem.Acquire(ctx, 5)
	assert.Equal(t, nil, err)

	// Acquire 4
	err = sem.Acquire(ctx, 4)
	assert.Equal(t, nil, err)

	// Try to acquire 2
	assert.Equal(t, false, sem.TryAcquire(2))

	// Try to acquire 1
	assert.Equal(t, true, sem.TryAcquire(1))

	// Release 7
	sem.Release(7)

	// Try to acquire 7
	assert.Equal(t, true, sem.TryAcquire(7))
}

func TestWeightedWithMoreAcquirersThanReleasers(t *testing.T) {
	ctx := context.Background()
	sem := NewWeighted(6)

	goroutines := 12
	acquire := 2
	release := 5
	wg := sync.WaitGroup{}
	acquireSuccessCount := atomic.Uint64{}
	acquireFailCount := atomic.Uint64{}

	wg.Add(goroutines)

	// Launch goroutines that try to acquire the semaphore
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			if err := sem.Acquire(ctx, acquire); err != nil {
				acquireFailCount.Add(1)
			} else {
				acquireSuccessCount.Add(1)
			}

			if sem.Acquired() >= release {
				sem.Release(release)
			}
		}()
	}

	// Wait for goroutines to finish
	wg.Wait()

	assert.Equal(t, uint64(12), acquireSuccessCount.Load())
	assert.Equal(t, uint64(0), acquireFailCount.Load())
	assert.Equal(t, 4, sem.Acquired())
}

func TestWeightedAcquireWithInvalidWeights(t *testing.T) {
	ctx := context.Background()
	sem := NewWeighted(10)

	// Acquire 0
	err := sem.Acquire(ctx, 0)
	assert.Equal(t, "semaphore: weight 0 cannot be negative or zero", err.Error())

	// Try to acquire 0
	res := sem.TryAcquire(0)
	assert.Equal(t, false, res)

	// Acquire -1
	err = sem.Acquire(ctx, -1)
	assert.Equal(t, "semaphore: weight -1 cannot be negative or zero", err.Error())

	// Try to acquire -1
	res = sem.TryAcquire(-1)
	assert.Equal(t, false, res)

	// Acquire 11
	err = sem.Acquire(ctx, 11)
	assert.Equal(t, "semaphore: weight 11 is greater than semaphore size 10", err.Error())

	// Try to acquire 11
	res = sem.TryAcquire(11)
	assert.Equal(t, false, res)
}

func TestWeightedReleaseWithInvalidWeights(t *testing.T) {
	sem := NewWeighted(10)

	// Release 0
	err := sem.Release(0)
	assert.Equal(t, "semaphore: weight 0 cannot be negative or zero", err.Error())

	// Release -1
	err = sem.Release(-1)
	assert.Equal(t, "semaphore: weight -1 cannot be negative or zero", err.Error())

	// Release 11
	err = sem.Release(11)
	assert.Equal(t, "semaphore: weight 11 is greater than semaphore size 10", err.Error())

	// Release 1
	err = sem.Release(1)
	assert.Equal(t, "semaphore: trying to release more than acquired: 1 > 0", err.Error())
}

func TestWeightedWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	sem := NewWeighted(10)

	// Acquire the semaphore
	err := sem.Acquire(ctx, 5)
	assert.Equal(t, nil, err)

	// Cancel the context
	cancel()

	// Attempt to acquire the semaphore
	err = sem.Acquire(ctx, 5)
	assert.Equal(t, context.Canceled, err)

	// Try to acquire the semaphore
	assert.Equal(t, false, sem.TryAcquire(5))
}

func TestWeightedWithContextCanceledWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	sem := NewWeighted(10)

	writers := 30
	wg := sync.WaitGroup{}
	wg.Add(writers)

	assert.Equal(t, 10, sem.Size())
	assert.Equal(t, 0, sem.Acquired())
	assert.Equal(t, 10, sem.Available())

	// Acquire the semaphore more than the semaphore size
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			sem.Acquire(ctx, 1)
		}()
	}

	// Wait until 10 goroutines are blocked
	for sem.Acquired() < 10 {
		time.Sleep(1 * time.Millisecond)
	}

	assert.Equal(t, 10, sem.Acquired())
	assert.Equal(t, 0, sem.Available())

	// Release 10 goroutines
	err := sem.Release(10)
	assert.Equal(t, nil, err)

	// Wait until 10 goroutines are blocked
	for sem.Acquired() < 10 {
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel the context
	cancel()

	// Wait for goroutines to finish
	wg.Wait()

	assert.Equal(t, 10, sem.Acquired())
	assert.Equal(t, 0, sem.Available())
	assert.Equal(t, context.Canceled, sem.Acquire(ctx, 1))
	assert.Equal(t, false, sem.TryAcquire(1))
}
