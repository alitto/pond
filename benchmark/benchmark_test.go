package benchmark

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/alitto/pond"
	"github.com/gammazero/workerpool"
	"github.com/panjf2000/ants/v2"
)

type subject struct {
	name    string
	factory poolFactory
}

type poolSubmit func(func())
type poolTeardown func()
type poolFactory func() (poolSubmit, poolTeardown)

type workload struct {
	name         string
	userCount    int
	taskCount    int
	taskInterval time.Duration
	task         func()
}

var maxWorkers = 200000

var workloads = []workload{{
	name:         "1u-10Mt",
	userCount:    1,
	taskCount:    1000000,
	taskInterval: 0,
}, {
	name:         "100u-10Kt",
	userCount:    100,
	taskCount:    10000,
	taskInterval: 0,
}, {
	name:         "1Ku-1Kt",
	userCount:    1000,
	taskCount:    1000,
	taskInterval: 0,
}, {
	name:         "10Ku-100t",
	userCount:    10000,
	taskCount:    100,
	taskInterval: 0,
}, {
	name:         "1Mu-1t",
	userCount:    1000000,
	taskCount:    1,
	taskInterval: 0,
}}

var pondSubjects = []subject{
	{
		name: "Pond-Eager",
		factory: func() (poolSubmit, poolTeardown) {
			pool := pond.New(maxWorkers, 1000000, pond.Strategy(pond.Eager()))

			return pool.Submit, pool.StopAndWait
		},
	}, {
		name: "Pond-Balanced",
		factory: func() (poolSubmit, poolTeardown) {
			pool := pond.New(maxWorkers, 1000000, pond.Strategy(pond.Balanced()))

			return pool.Submit, pool.StopAndWait
		},
	}, {
		name: "Pond-Lazy",
		factory: func() (poolSubmit, poolTeardown) {
			pool := pond.New(maxWorkers, 1000000, pond.Strategy(pond.Lazy()))

			return pool.Submit, pool.StopAndWait
		},
	},
}

var otherSubjects = []subject{
	{
		name: "Goroutines",
		factory: func() (poolSubmit, poolTeardown) {
			submit := func(taskFunc func()) {
				go func() {
					taskFunc()
				}()
			}
			return submit, func() {}
		},
	},
	{
		name: "GoroutinePool",
		factory: func() (poolSubmit, poolTeardown) {

			var poolWg sync.WaitGroup
			taskChan := make(chan func())
			poolWg.Add(maxWorkers)
			for i := 0; i < maxWorkers; i++ {
				go func() {
					for task := range taskChan {
						task()
					}
					poolWg.Done()
				}()
			}

			submit := func(task func()) {
				taskChan <- task
			}
			teardown := func() {
				close(taskChan)
				poolWg.Wait()
			}

			return submit, teardown
		},
	},
	{
		name: "BufferedPool",
		factory: func() (poolSubmit, poolTeardown) {

			var poolWg sync.WaitGroup
			taskChan := make(chan func(), 1000000)
			poolWg.Add(maxWorkers)
			for i := 0; i < maxWorkers; i++ {
				go func() {
					for task := range taskChan {
						task()
					}
					poolWg.Done()
				}()
			}

			submit := func(task func()) {
				taskChan <- task
			}
			teardown := func() {
				close(taskChan)
				poolWg.Wait()
			}

			return submit, teardown
		},
	},
	{
		name: "Gammazero",
		factory: func() (poolSubmit, poolTeardown) {
			pool := workerpool.New(maxWorkers)
			return pool.Submit, pool.StopWait
		},
	},
	{
		name: "AntsPool",
		factory: func() (poolSubmit, poolTeardown) {
			pool, _ := ants.NewPool(maxWorkers, ants.WithExpiryDuration(10*time.Second))
			submit := func(task func()) {
				pool.Submit(task)
			}
			return submit, pool.Release
		},
	},
}

func BenchmarkPondSleep10ms(b *testing.B) {
	sleep10ms := func() {
		time.Sleep(10 * time.Millisecond)
	}
	runBenchmarks(b, workloads, pondSubjects, sleep10ms)
}

func BenchmarkPondRandFloat64(b *testing.B) {
	randFloat64 := func() {
		rand.Float64()
	}
	runBenchmarks(b, workloads, pondSubjects, randFloat64)
}

func BenchmarkAllSleep10ms(b *testing.B) {
	subjects := make([]subject, 0)
	subjects = append(subjects, pondSubjects...)
	subjects = append(subjects, otherSubjects...)
	sleep10ms := func() {
		time.Sleep(10 * time.Millisecond)
	}
	runBenchmarks(b, workloads, subjects, sleep10ms)
}

func BenchmarkAllRandFloat64(b *testing.B) {
	subjects := make([]subject, 0)
	subjects = append(subjects, pondSubjects...)
	subjects = append(subjects, otherSubjects...)
	randFloat64 := func() {
		rand.Float64()
	}
	runBenchmarks(b, workloads, subjects, randFloat64)
}

func runBenchmarks(b *testing.B, workloads []workload, subjects []subject, task func()) {
	for _, workload := range workloads {
		for _, subject := range subjects {
			testName := fmt.Sprintf("%s/%s", workload.name, subject.name)
			b.Run(testName, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					simulateWorkload(&workload, subject.factory, task)
				}
			})
		}
	}
}

func simulateWorkload(workload *workload, poolFactoy poolFactory, task func()) {

	// Create pool
	poolSubmit, poolTeardown := poolFactoy()

	// Spawn one goroutine per simulated user
	var wg sync.WaitGroup
	wg.Add(workload.userCount * workload.taskCount)

	testFunc := func() {
		task()
		wg.Done()
	}

	for i := 0; i < workload.userCount; i++ {
		go func() {
			// Every user submits tasksPerUser at the specified frequency
			for i := 0; i < workload.taskCount; i++ {
				poolSubmit(testFunc)
				if workload.taskInterval > 0 {
					time.Sleep(workload.taskInterval)
				}
			}
		}()
	}
	wg.Wait()

	// Tear down
	poolTeardown()
}
