package pond_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond"
)

func assertEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

func assertNotEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected == actual {
		t.Helper()
		t.Errorf("Expected not equal to %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

func TestSubmitAndStopWait(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit tasks
	var doneCount int32
	for i := 0; i < 17; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
		})
	}

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, int32(17), atomic.LoadInt32(&doneCount))
}

func TestSubmitAndStopWaitFor(t *testing.T) {

	pool := pond.New(1, 10)

	// Submit a long running task
	var doneCount int32
	pool.Submit(func() {
		time.Sleep(2 * time.Second)
		atomic.AddInt32(&doneCount, 1)
	})

	// Wait 100ms for the task to complete
	pool.StopAndWaitFor(50 * time.Millisecond)

	assertEqual(t, int32(0), atomic.LoadInt32(&doneCount))
}

func TestSubmitAndStopWaitForWithEnoughDeadline(t *testing.T) {

	pool := pond.New(1, 10)

	// Submit tasks
	var doneCount int32
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
		})
	}

	// Wait until all submitted tasks complete
	pool.StopAndWaitFor(1 * time.Second)

	assertEqual(t, int32(10), atomic.LoadInt32(&doneCount))
}

func TestSubmitAndStopWaitingWithMoreWorkersThanTasks(t *testing.T) {

	pool := pond.New(18, 5)

	// Submit tasks
	var doneCount int32
	for i := 0; i < 17; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
		})
	}

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, int32(17), atomic.LoadInt32(&doneCount))
}

func TestSubmitAndStopWithoutWaiting(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit tasks
	started := make(chan bool)
	completed := make(chan bool)
	var doneCount int32
	for i := 0; i < 5; i++ {
		pool.Submit(func() {
			started <- true
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
			<-completed
		})
	}

	// Make sure the first task started
	<-started

	// Stop without waiting for the rest of the tasks to start
	ctx := pool.Stop()

	// Let the first task complete now
	completed <- true

	// Only the first task should have been completed, the rest are discarded
	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))

	// Make sure the exit lines in the worker pool are executed and covered
	<-ctx.Done()
}

func TestSubmitWithNilTask(t *testing.T) {

	pool := pond.New(2, 5)

	// Submit nil task
	pool.Submit(nil)

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestSubmitAndWait(t *testing.T) {

	pool := pond.New(1, 5)
	defer pool.StopAndWait()

	// Submit a task and wait for it to complete
	var doneCount int32
	pool.SubmitAndWait(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	})

	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))
}

func TestSubmitAndWaitWithNilTask(t *testing.T) {

	pool := pond.New(2, 5)

	// Submit nil task
	pool.SubmitAndWait(nil)

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestSubmitBefore(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit a long-running task
	var doneCount int32
	pool.Submit(func() {
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	})

	// Submit a task that times out after 5ms
	pool.SubmitBefore(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	}, 5*time.Millisecond)

	// Submit a task that times out after 1s
	pool.SubmitBefore(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	}, 1*time.Second)

	pool.StopAndWait()

	// Only 2 tasks must have executed
	assertEqual(t, int32(2), atomic.LoadInt32(&doneCount))
}

func TestSubmitBeforeWithNilTask(t *testing.T) {

	pool := pond.New(3, 5)

	// Submit nil task
	pool.SubmitBefore(nil, 1*time.Millisecond)

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestTrySubmit(t *testing.T) {

	pool := pond.New(1, 0)

	// Submit a long-running task
	var doneCount int32
	pool.Submit(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	})

	// Attempt to submit a task without blocking
	dispatched := pool.TrySubmit(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	})

	// Task was not dispatched because the pool was full
	assertEqual(t, false, dispatched)

	pool.StopAndWait()

	// Only the first task must have executed
	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))
}

func TestTrySubmitOnStoppedPool(t *testing.T) {

	// Create a pool and stop it immediately
	pool := pond.New(1, 0)
	assertEqual(t, false, pool.Stopped())
	pool.StopAndWait()
	assertEqual(t, true, pool.Stopped())

	submitted := pool.TrySubmit(func() {})

	// Task should not be accepted by the pool
	assertEqual(t, false, submitted)
}

func TestSubmitToIdle(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit a task and wait for it to complete
	pool.SubmitAndWait(func() {
		time.Sleep(1 * time.Millisecond)
	})

	time.Sleep(10 * time.Millisecond)

	assertEqual(t, int(1), pool.RunningWorkers())
	assertEqual(t, int(1), pool.IdleWorkers())

	// Submit another task (this one should go to the idle worker)
	pool.SubmitAndWait(func() {
		time.Sleep(1 * time.Millisecond)
	})

	pool.StopAndWait()

	time.Sleep(10 * time.Millisecond)

	assertEqual(t, int(0), pool.RunningWorkers())
	assertEqual(t, int(0), pool.IdleWorkers())
}

func TestSubmitOnStoppedPool(t *testing.T) {

	// Create a pool and stop it immediately
	pool := pond.New(1, 0)
	assertEqual(t, false, pool.Stopped())
	pool.StopAndWait()
	assertEqual(t, true, pool.Stopped())

	// Attempt to submit a task on a stopped pool
	var err interface{} = nil
	func() {
		defer func() {
			err = recover()
		}()
		pool.Submit(func() {})
	}()

	// Call to Submit should have failed with ErrSubmitOnStoppedPool error
	assertEqual(t, pond.ErrSubmitOnStoppedPool, err)
}

func TestRunning(t *testing.T) {

	workerCount := 5
	taskCount := 10
	pool := pond.New(workerCount, taskCount)

	assertEqual(t, 0, pool.RunningWorkers())

	// Submit tasks
	var started = make(chan struct{}, workerCount)
	var completed = make(chan struct{}, workerCount)
	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			started <- struct{}{}
			time.Sleep(1 * time.Millisecond)
			<-completed
		})
	}

	// Wait until half the tasks have started
	for i := 0; i < taskCount/2; i++ {
		<-started
	}

	assertEqual(t, workerCount, pool.RunningWorkers())
	time.Sleep(1 * time.Millisecond)

	// Make sure half the tasks tasks complete
	for i := 0; i < taskCount/2; i++ {
		completed <- struct{}{}
	}

	// Wait until the rest of the tasks have started
	for i := 0; i < taskCount/2; i++ {
		<-started
	}

	// Make sure all tasks complete
	for i := 0; i < taskCount/2; i++ {
		completed <- struct{}{}
	}

	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestSubmitWithPanic(t *testing.T) {

	pool := pond.New(1, 5)
	assertEqual(t, 0, pool.RunningWorkers())

	// Submit a task that panics
	var doneCount int32
	pool.Submit(func() {
		arr := make([]string, 0)
		fmt.Printf("Out of range value: %s", arr[1])
		atomic.AddInt32(&doneCount, 1)
	})

	// Submit a task that completes normally
	pool.Submit(func() {
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	})

	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))
}

func TestPoolWithCustomIdleTimeout(t *testing.T) {

	pool := pond.New(1, 5, pond.IdleTimeout(1*time.Millisecond))

	// Submit a task
	started := make(chan bool)
	completed := make(chan bool)
	pool.Submit(func() {
		<-started
		time.Sleep(3 * time.Millisecond)
		<-completed
	})

	// Make sure the first task has started
	started <- true

	// There should be 1 worker running
	assertEqual(t, 1, pool.RunningWorkers())

	// Let the task complete
	completed <- true

	// Wait for some time
	time.Sleep(50 * time.Millisecond)

	// Worker should have been killed
	assertEqual(t, 0, pool.RunningWorkers())

	pool.StopAndWait()
}

func TestPoolWithCustomPanicHandler(t *testing.T) {

	var capturedPanic interface{} = nil
	panicHandler := func(panic interface{}) {
		capturedPanic = panic
	}

	pool := pond.New(1, 5, pond.PanicHandler(panicHandler))

	// Submit a task that panics
	pool.Submit(func() {
		panic("panic now!")
	})

	pool.StopAndWait()

	// Panic should have been captured
	assertEqual(t, "panic now!", capturedPanic)
}

func TestPoolWithCustomMinWorkers(t *testing.T) {

	pool := pond.New(10, 5, pond.MinWorkers(10))

	// Submit a task that panics
	started := make(chan struct{})
	completed := make(chan struct{})
	pool.Submit(func() {
		<-started
		completed <- struct{}{}
	})

	started <- struct{}{}

	// 10 workers should have been started
	assertEqual(t, 10, pool.RunningWorkers())

	<-completed

	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestPoolWithCustomStrategy(t *testing.T) {

	pool := pond.New(3, 3, pond.Strategy(pond.RatedResizer(2)))

	// Submit 3 tasks
	group := pool.Group()
	for i := 0; i < 3; i++ {
		group.Submit(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Wait for them to complete
	group.Wait()

	// 2 workers should have been started
	assertEqual(t, 2, pool.RunningWorkers())

	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
}

func TestMetricsAndGetters(t *testing.T) {

	pool := pond.New(5, 10)
	assertEqual(t, 0, pool.RunningWorkers())

	// Submit successful tasks
	for i := 0; i < 18; i++ {
		pool.TrySubmit(func() {
			time.Sleep(5 * time.Millisecond)
		})
	}

	// Submit a task that panics
	pool.Submit(func() {
		arr := make([]string, 0)
		fmt.Printf("Out of range value: %s", arr[1])
	})

	// Submit another successful task
	pool.Submit(func() {
		time.Sleep(1 * time.Millisecond)
	})

	// Submit a nil task (it should not update counters)
	pool.Submit(nil)

	pool.StopAndWait()

	assertEqual(t, 0, pool.RunningWorkers())
	assertEqual(t, 0, pool.MinWorkers())
	assertEqual(t, 5, pool.MaxWorkers())
	assertEqual(t, 10, pool.MaxCapacity())
	assertNotEqual(t, nil, pool.Strategy())
	assertEqual(t, uint64(17), pool.SubmittedTasks())
	assertEqual(t, uint64(16), pool.SuccessfulTasks())
	assertEqual(t, uint64(1), pool.FailedTasks())
	assertEqual(t, uint64(17), pool.CompletedTasks())
	assertEqual(t, uint64(0), pool.WaitingTasks())
}

func TestSubmitWithContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := pond.New(1, 5, pond.Context(ctx))

	var doneCount, taskCount int32

	// Submit a long-running, cancellable task
	pool.Submit(func() {
		atomic.AddInt32(&taskCount, 1)
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Minute):
			atomic.AddInt32(&doneCount, 1)
			return
		}
	})

	// Cancel the context
	cancel()

	pool.StopAndWait()

	assertEqual(t, int32(1), atomic.LoadInt32(&taskCount))
	assertEqual(t, int32(0), atomic.LoadInt32(&doneCount))
}

func TestConcurrentStopAndWait(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit tasks
	var doneCount int32
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
		})
	}

	wg := sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			pool.StopAndWait()
			assertEqual(t, int32(10), atomic.LoadInt32(&doneCount))
		}()
	}

	wg.Wait()
}

func TestSubmitToIdleWorker(t *testing.T) {

	pool := pond.New(6, 0, pond.MinWorkers(3))

	assertEqual(t, 3, pool.RunningWorkers())

	// Submit task
	var doneCount int32
	for i := 0; i < 3; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
			atomic.AddInt32(&doneCount, 1)
		})
	}

	// Verify no new workers were started
	assertEqual(t, 3, pool.RunningWorkers())

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, int32(3), atomic.LoadInt32(&doneCount))
}
