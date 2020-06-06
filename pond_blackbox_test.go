package pond_test

import (
	"fmt"
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

func TestSubmitAndStopWaiting(t *testing.T) {

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
	pool.Stop()

	// Let the first task complete now
	completed <- true

	// Only the first task should have been completed, the rest are discarded
	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))

	// Make sure the exit lines in the worker pool are executed and covered
	time.Sleep(6 * time.Millisecond)
}

func TestSubmitWithNilTask(t *testing.T) {

	pool := pond.New(2, 5)

	// Submit nil task
	pool.Submit(nil)

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, 0, pool.Running())
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

	assertEqual(t, 0, pool.Running())
}

func TestSubmitBefore(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit a long-running task
	var doneCount int32
	pool.SubmitBefore(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	}, 1*time.Millisecond)

	// Submit a task that times out after 2ms
	pool.SubmitBefore(func() {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&doneCount, 1)
	}, 2*time.Millisecond)

	pool.StopAndWait()

	// Only the first task must have executed
	assertEqual(t, int32(1), atomic.LoadInt32(&doneCount))
}

func TestSubmitBeforeWithNilTask(t *testing.T) {

	pool := pond.New(3, 5)

	// Submit nil task
	pool.SubmitBefore(nil, 1*time.Millisecond)

	// Wait until all submitted tasks complete
	pool.StopAndWait()

	assertEqual(t, 0, pool.Running())
}

func TestTrySubmit(t *testing.T) {

	pool := pond.New(1, 5)

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

func TestSubmitToIdle(t *testing.T) {

	pool := pond.New(1, 5)

	// Submit a task and wait for it to complete
	pool.SubmitAndWait(func() {
		time.Sleep(1 * time.Millisecond)
	})

	assertEqual(t, int(1), pool.Running())
	assertEqual(t, int(1), pool.Idle())

	// Submit another task (this one should go to the idle worker)
	pool.SubmitAndWait(func() {
		time.Sleep(1 * time.Millisecond)
	})

	pool.StopAndWait()

	assertEqual(t, int(0), pool.Running())
	assertEqual(t, int(0), pool.Idle())
}

func TestRunning(t *testing.T) {

	workerCount := 5
	taskCount := 10
	pool := pond.New(workerCount, taskCount)

	assertEqual(t, 0, pool.Running())

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

	assertEqual(t, workerCount, pool.Running())
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

	assertEqual(t, 0, pool.Running())
}

func TestSubmitWithPanic(t *testing.T) {

	pool := pond.New(1, 5)
	assertEqual(t, 0, pool.Running())

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

	assertEqual(t, 0, pool.Running())
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
	assertEqual(t, 1, pool.Running())

	// Let the task complete
	completed <- true

	// Wait for some time
	time.Sleep(10 * time.Millisecond)

	// Worker should have been killed
	assertEqual(t, 0, pool.Running())

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
	assertEqual(t, 10, pool.Running())

	<-completed

	pool.StopAndWait()

	assertEqual(t, 0, pool.Running())
}

func TestGroupSubmit(t *testing.T) {

	pool := pond.New(5, 1000)
	assertEqual(t, 0, pool.Running())

	// Submit groups of tasks
	var doneCount, taskCount int32
	var groups []*pond.TaskGroup
	for i := 0; i < 5; i++ {
		group := pool.Group()
		for j := 0; j < i+5; j++ {
			group.Submit(func() {
				time.Sleep(1 * time.Millisecond)
				atomic.AddInt32(&doneCount, 1)
			})
			taskCount++
		}
		groups = append(groups, group)
	}

	// Wait for all groups to complete
	for _, group := range groups {
		group.Wait()
	}

	assertEqual(t, int32(taskCount), atomic.LoadInt32(&doneCount))
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
	assertEqual(t, 2, pool.Running())

	pool.StopAndWait()

	assertEqual(t, 0, pool.Running())
}
