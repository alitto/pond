package pond_test

import (
	"sync"
	"testing"
	"time"

	"github.com/alitto/pond"
	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
)

const (
	taskCount    = 1000000
	taskDuration = 10 * time.Millisecond
	workerCount  = 200000
)

func BenchmarkPond(b *testing.B) {
	var wg sync.WaitGroup
	pool := pond.New(workerCount, taskCount)
	defer pool.StopAndWait()

	// Submit tasks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			pool.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
	}
	b.StopTimer()
}

func BenchmarkPondGroup(b *testing.B) {
	pool := pond.New(workerCount, taskCount)
	defer pool.StopAndWait()

	// Submit tasks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := pool.Group()
		for i := 0; i < taskCount; i++ {
			group.Submit(func() {
				time.Sleep(taskDuration)
			})
		}
		group.Wait()
	}
	b.StopTimer()
}

func BenchmarkGoroutines(b *testing.B) {
	var wg sync.WaitGroup

	// Submit tasks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			go func() {
				time.Sleep(taskDuration)
				wg.Done()
			}()
		}
		wg.Wait()
	}
	b.StopTimer()
}

func BenchmarkGammazeroWorkerpool(b *testing.B) {
	var wg sync.WaitGroup
	wp := workerpool.New(workerCount)
	defer wp.StopWait()

	// Submit tasks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			wp.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
	}
	b.StopTimer()
}

func BenchmarkAnts(b *testing.B) {
	var wg sync.WaitGroup
	p, _ := ants.NewPool(workerCount, ants.WithExpiryDuration(10*time.Second))
	defer p.Release()

	// Submit tasks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			_ = p.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
	}
	b.StopTimer()
}
