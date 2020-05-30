package benchmark

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alitto/pond"
	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
)

type workload struct {
	name         string
	taskCount    int
	taskDuration time.Duration
}

type subject struct {
	name   string
	test   poolTest
	config poolConfig
}

type poolConfig struct {
	minWorkers  int
	maxWorkers  int
	maxCapacity int
	strategy    pond.ResizingStrategy
}

type poolTest func(taskCount int, taskFunc func(), config poolConfig)

var workloads = []workload{
	{"1M-10ms", 1000000, 10 * time.Millisecond},
	{"100k-500ms", 100000, 500 * time.Millisecond},
	{"10k-1000ms", 10000, 1000 * time.Millisecond},
}

var defaultPoolConfig = poolConfig{
	maxWorkers: 200000,
}

var pondSubjects = []subject{
	{"Pond-Eager", pondPool, poolConfig{maxWorkers: defaultPoolConfig.maxWorkers, maxCapacity: 1000000, strategy: pond.Eager()}},
	{"Pond-Balanced", pondPool, poolConfig{maxWorkers: defaultPoolConfig.maxWorkers, maxCapacity: 1000000, strategy: pond.Balanced()}},
	{"Pond-Lazy", pondPool, poolConfig{maxWorkers: defaultPoolConfig.maxWorkers, maxCapacity: 1000000, strategy: pond.Lazy()}},
}

var otherSubjects = []subject{
	{"Goroutines", unboundedGoroutines, defaultPoolConfig},
	{"GoroutinePool", goroutinePool, defaultPoolConfig},
	{"BufferedPool", bufferedGoroutinePool, defaultPoolConfig},
	{"Gammazero", gammazeroWorkerpool, defaultPoolConfig},
	{"AntsPool", antsPool, defaultPoolConfig},
}

func BenchmarkPond(b *testing.B) {
	runBenchmarks(b, workloads, pondSubjects)
}

func BenchmarkAll(b *testing.B) {
	allSubjects := make([]subject, 0)
	allSubjects = append(allSubjects, pondSubjects...)
	allSubjects = append(allSubjects, otherSubjects...)
	runBenchmarks(b, workloads, allSubjects)
}

func runBenchmarks(b *testing.B, workloads []workload, subjects []subject) {
	for _, workload := range workloads {
		taskFunc := func() {
			time.Sleep(workload.taskDuration)
		}
		for _, subject := range subjects {
			name := fmt.Sprintf("%s/%s", workload.name, subject.name)
			b.Run(name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					subject.test(workload.taskCount, taskFunc, subject.config)
				}
			})
		}
	}
}

func pondPool(taskCount int, taskFunc func(), config poolConfig) {
	var wg sync.WaitGroup
	pool := pond.New(config.maxWorkers, config.maxCapacity,
		pond.MinWorkers(config.minWorkers),
		pond.Strategy(config.strategy))
	// Submit tasks
	wg.Add(taskCount)
	for n := 0; n < taskCount; n++ {
		pool.Submit(func() {
			taskFunc()
			wg.Done()
		})
	}
	wg.Wait()
	pool.StopAndWait()
}

func unboundedGoroutines(taskCount int, taskFunc func(), config poolConfig) {
	var wg sync.WaitGroup
	wg.Add(taskCount)
	for i := 0; i < taskCount; i++ {
		go func() {
			taskFunc()
			wg.Done()
		}()
	}
	wg.Wait()
}

func goroutinePool(taskCount int, taskFunc func(), config poolConfig) {
	// Start worker goroutines
	var poolWg sync.WaitGroup
	taskChan := make(chan func())
	poolWg.Add(config.maxWorkers)
	for i := 0; i < config.maxWorkers; i++ {
		go func() {
			for task := range taskChan {
				task()
			}
			poolWg.Done()
		}()
	}

	// Submit tasks and wait for completion
	var wg sync.WaitGroup
	wg.Add(taskCount)
	for i := 0; i < taskCount; i++ {
		taskChan <- func() {
			taskFunc()
			wg.Done()
		}
	}
	close(taskChan)
	wg.Wait()
	poolWg.Wait()
}

func bufferedGoroutinePool(taskCount int, taskFunc func(), config poolConfig) {
	// Start worker goroutines
	var poolWg sync.WaitGroup
	taskChan := make(chan func(), taskCount)
	poolWg.Add(config.maxWorkers)
	for i := 0; i < config.maxWorkers; i++ {
		go func() {
			for task := range taskChan {
				task()
			}
			poolWg.Done()
		}()
	}

	// Submit tasks and wait for completion
	var wg sync.WaitGroup
	wg.Add(taskCount)
	for i := 0; i < taskCount; i++ {
		taskChan <- func() {
			taskFunc()
			wg.Done()
		}
	}
	close(taskChan)
	wg.Wait()
	poolWg.Wait()
}

func gammazeroWorkerpool(taskCount int, taskFunc func(), config poolConfig) {
	// Create pool
	wp := workerpool.New(config.maxWorkers)
	defer wp.StopWait()

	// Submit tasks and wait for completion
	var wg sync.WaitGroup
	wg.Add(taskCount)
	for i := 0; i < taskCount; i++ {
		wp.Submit(func() {
			taskFunc()
			wg.Done()
		})
	}
	wg.Wait()
}

func antsPool(taskCount int, taskFunc func(), config poolConfig) {
	// Create pool
	pool, _ := ants.NewPool(config.maxWorkers, ants.WithExpiryDuration(10*time.Second))
	defer pool.Release()

	// Submit tasks and wait for completion
	var wg sync.WaitGroup
	wg.Add(taskCount)
	for i := 0; i < taskCount; i++ {
		_ = pool.Submit(func() {
			taskFunc()
			wg.Done()
		})
	}
	wg.Wait()
}
