package pond

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestSubpool(t *testing.T) {
	taskDuration := 10 * time.Millisecond
	taskCount := 10
	maxConcurrency := 5

	pool := NewPool(10)
	subpool := pool.NewSubpool(maxConcurrency)
	subsubpool := subpool.NewSubpool(1)

	completed := make(chan struct{})
	var subpoolElapsedTime time.Duration
	var subsubpoolElapsedTime time.Duration

	go func() {
		start := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			subpool.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
		subpoolElapsedTime = time.Since(start)
		completed <- struct{}{}
	}()

	go func() {
		start := time.Now()
		wg := sync.WaitGroup{}
		wg.Add(taskCount)
		for i := 0; i < taskCount; i++ {
			subsubpool.Submit(func() {
				time.Sleep(taskDuration)
				wg.Done()
			})
		}
		wg.Wait()
		subsubpool.Stop().Wait()
		subsubpoolElapsedTime = time.Since(start)
		completed <- struct{}{}
	}()

	<-completed
	<-completed

	assert.True(t, subpoolElapsedTime >= time.Duration(taskCount/maxConcurrency)*taskDuration)
	assert.True(t, subsubpoolElapsedTime >= time.Duration(taskCount/1)*taskDuration)
}

func TestSubpoolStopAndWait(t *testing.T) {
	taskDuration := 1 * time.Microsecond
	taskCount := 100000
	maxConcurrency := 100

	subpool := NewPool(1000).NewSubpool(maxConcurrency)

	var executedCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		subpool.Submit(func() {
			time.Sleep(taskDuration)
			executedCount.Add(1)
		})
	}

	subpool.StopAndWait()

	assert.Equal(t, int64(taskCount), executedCount.Load())
}

func TestSubpoolMetrics(t *testing.T) {
	pool := NewPool(10)
	subpool := pool.NewSubpool(5)

	sampleErr := errors.New("sample error")

	for i := 0; i < 20; i++ {
		i := i
		if i%2 == 0 {
			subpool.SubmitErr(func() error {
				if i%4 == 0 {
					return sampleErr
				}

				return nil
			})
		} else {
			pool.SubmitErr(func() error {
				if i%3 == 0 {
					return sampleErr
				}

				return nil
			})
		}
	}

	subpool.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, uint64(20), pool.SubmittedTasks())
	assert.Equal(t, uint64(20), pool.CompletedTasks())
	assert.Equal(t, uint64(8), pool.FailedTasks())
	assert.Equal(t, uint64(12), pool.SuccessfulTasks())
	assert.Equal(t, uint64(0), pool.WaitingTasks())

	assert.Equal(t, uint64(10), subpool.SubmittedTasks())
	assert.Equal(t, uint64(10), subpool.CompletedTasks())
	assert.Equal(t, uint64(5), subpool.FailedTasks())
	assert.Equal(t, uint64(5), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())

}

func TestSubpoolStop(t *testing.T) {
	pool := NewPool(10)
	subpool := pool.NewSubpool(5)

	var executedCount atomic.Int64

	subpool.Submit(func() {
		executedCount.Add(1)
	})

	subpool.StopAndWait()

	pool.Submit(func() {
		executedCount.Add(1)
	}).Wait()

	assert.Equal(t, int64(2), executedCount.Load())

	err := subpool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	pool.StopAndWait()

	err = pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestSubpoolMaxConcurrency(t *testing.T) {
	pool := NewPool(10)

	assert.PanicsWithError(t, "maxConcurrency must be greater than or equal to 0", func() {
		pool.NewSubpool(-1)
	})

	assert.PanicsWithError(t, "maxConcurrency cannot be greater than the parent pool's maxConcurrency (10)", func() {
		pool.NewSubpool(11)
	})

	subpool := pool.NewSubpool(0)

	assert.Equal(t, 10, subpool.MaxConcurrency())
}

func TestSubpoolStoppedAfterCancel(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	pool := NewPool(10, WithContext(ctx))
	subpool := pool.NewSubpool(5)

	cancel()

	time.Sleep(1 * time.Millisecond)

	err := pool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)

	err = subpool.Submit(func() {}).Wait()

	assert.Equal(t, ErrPoolStopped, err)
}

func TestSubpoolStopAndWaitShouldNotDeadlockWhenContextCanceledWithQueuedTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parentPool := NewPool(1, WithContext(ctx))
	subpool := parentPool.NewSubpool(1)

	firstTaskStarted := make(chan struct{})
	releaseFirstTask := make(chan struct{})

	firstTask := subpool.Submit(func() {
		close(firstTaskStarted)
		<-releaseFirstTask
	})

	// This task stays queued in the subpool while the only worker is blocked.
	secondTask := subpool.Submit(func() {})

	<-firstTaskStarted
	cancel()
	close(releaseFirstTask)

	stopDone := make(chan struct{})
	go func() {
		subpool.StopAndWait()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("subpool.StopAndWait blocked after context cancellation with queued tasks")
	}

	assert.Equal(t, context.Canceled, firstTask.Wait())
	assert.Equal(t, context.Canceled, secondTask.Wait())

	parentPool.StopAndWait()

	assert.Equal(t, uint64(2), subpool.SubmittedTasks())
	assert.Equal(t, uint64(1), subpool.CompletedTasks())
	assert.Equal(t, uint64(1), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.FailedTasks())
	assert.Equal(t, uint64(1), subpool.CanceledTasks())
	assert.Equal(t, uint64(0), subpool.DroppedTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())
	assert.Equal(t, int64(0), subpool.RunningWorkers())

	assert.Equal(t, uint64(2), parentPool.SubmittedTasks())
	assert.Equal(t, uint64(1), parentPool.CompletedTasks())
	assert.Equal(t, uint64(1), parentPool.SuccessfulTasks())
	assert.Equal(t, uint64(0), parentPool.FailedTasks())
	assert.Equal(t, uint64(1), parentPool.CanceledTasks())
	assert.Equal(t, uint64(1), parentPool.DroppedTasks())
	assert.Equal(t, uint64(0), parentPool.WaitingTasks())
	assert.Equal(t, int64(0), parentPool.RunningWorkers())
}

func TestSubpoolWithDifferentLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewPool(7, WithContext(ctx))

	subpool1 := pool.NewSubpool(1)
	subpool2 := pool.NewSubpool(2)
	subpool3 := pool.NewSubpool(3)

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	var task = func() func() {
		return func() {
			taskStarted <- struct{}{}
			<-taskWait
		}
	}

	// Submit tasks to subpool1 and wait for 1 task to start
	for i := 0; i < 10; i++ {
		subpool1.Submit(task())
	}
	<-taskStarted

	// Submit tasks to subpool2 and wait for 2 tasks to start
	for i := 0; i < 10; i++ {
		subpool2.Submit(task())
	}
	<-taskStarted
	<-taskStarted

	// Submit tasks to subpool3 and wait for 3 tasks to start
	for i := 0; i < 10; i++ {
		subpool3.Submit(task())
	}
	<-taskStarted
	<-taskStarted
	<-taskStarted

	// Submit tasks to the main pool and wait for 1 to start
	for i := 0; i < 10; i++ {
		pool.Submit(task())
	}
	<-taskStarted

	// Verify concurrency of each pool
	assert.Equal(t, int64(1), subpool1.RunningWorkers())
	assert.Equal(t, int64(2), subpool2.RunningWorkers())
	assert.Equal(t, int64(3), subpool3.RunningWorkers())
	assert.Equal(t, int64(7), pool.RunningWorkers())

	assert.Equal(t, uint64(0), subpool1.CompletedTasks())
	assert.Equal(t, uint64(0), subpool2.CompletedTasks())
	assert.Equal(t, uint64(0), subpool3.CompletedTasks())
	assert.Equal(t, uint64(0), pool.CompletedTasks())

	// Cancel the context to abort pending tasks
	cancel()

	// Unblock all running tasks
	close(taskWait)

	subpool1.StopAndWait()
	subpool2.StopAndWait()
	subpool3.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, uint64(1), subpool1.CompletedTasks())
	assert.Equal(t, uint64(2), subpool2.CompletedTasks())
	assert.Equal(t, uint64(3), subpool3.CompletedTasks())
	assert.Equal(t, uint64(7), pool.CompletedTasks())
}

func TestSubpoolWithOverlappingConcurrency(t *testing.T) {
	taskDuration := 1 * time.Millisecond

	pool := NewPool(1)
	subpool := pool.NewSubpool(1)

	var executedCount atomic.Int64

	subpool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	pool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	subpool.Submit(func() {
		time.Sleep(taskDuration)
		executedCount.Add(1)
		assert.Equal(t, int64(1), pool.RunningWorkers())
		assert.Equal(t, int64(1), subpool.RunningWorkers())
	})

	subpool.StopAndWait()
	pool.StopAndWait()

	assert.Equal(t, int64(3), executedCount.Load())
}

func TestSubpoolWithQueueSizeOverride(t *testing.T) {
	pool := NewPool(10, WithQueueSize(10))

	subpool := pool.NewSubpool(1, WithQueueSize(2), WithNonBlocking(true))

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{})

	var task = func() func() {
		return func() {
			taskStarted <- struct{}{}
			<-taskWait
		}
	}

	// Submit tasks to subpool and wait for it to start
	subpool.Submit(task())
	<-taskStarted

	// Submit more tasks to fill up the queue
	for i := 0; i < 10; i++ {
		subpool.Submit(task())
	}

	assert.Equal(t, int64(1), subpool.RunningWorkers())
	assert.Equal(t, uint64(11), subpool.SubmittedTasks())
	assert.Equal(t, uint64(0), subpool.CompletedTasks())
	assert.Equal(t, uint64(8), subpool.DroppedTasks())
	assert.Equal(t, uint64(2), subpool.WaitingTasks())

	// Unblock all running tasks
	close(taskWait)

	subpool.StopAndWait()
	pool.StopAndWait()
}

func TestSubpoolResize(t *testing.T) {

	parentPool := NewPool(10, WithQueueSize(10))

	pool := parentPool.NewSubpool(1)

	assert.Equal(t, 1, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

	taskStarted := make(chan struct{}, 10)
	taskWait := make(chan struct{}, 10)

	// Submit 10 tasks
	for i := 0; i < 10; i++ {
		pool.Submit(func() {
			<-taskStarted
			<-taskWait
		})
	}

	// Unblock 3 tasks
	for i := 0; i < 3; i++ {
		taskStarted <- struct{}{}
	}

	// Verify only 1 task is running and 9 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(9), pool.WaitingTasks())
	assert.Equal(t, int64(1), pool.RunningWorkers())
	assert.Equal(t, int64(1), parentPool.RunningWorkers())

	// Increase max concurrency to 3
	pool.Resize(3)
	assert.Equal(t, 3, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

	// Unblock 3 more tasks
	for i := 0; i < 3; i++ {
		taskStarted <- struct{}{}
	}

	// Verify 3 tasks are running and 7 are waiting
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, uint64(7), pool.WaitingTasks())
	assert.Equal(t, int64(3), pool.RunningWorkers())
	assert.Equal(t, int64(3), parentPool.RunningWorkers())

	// Decrease max concurrency to 1
	pool.Resize(2)
	assert.Equal(t, 2, pool.MaxConcurrency())
	assert.Equal(t, 10, parentPool.MaxConcurrency())

	// Complete the 3 running tasks
	for i := 0; i < 3; i++ {
		taskWait <- struct{}{}
	}

	// Unblock all remaining tasks
	for i := 0; i < 4; i++ {
		taskStarted <- struct{}{}
	}

	// Ensure 2 tasks are running and 5 are waiting
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, uint64(5), pool.WaitingTasks())
	assert.Equal(t, int64(2), pool.RunningWorkers())
	assert.Equal(t, int64(2), parentPool.RunningWorkers())

	close(taskWait)

	pool.Stop().Wait()
}

func TestSubpoolRecoversAfterGoexit(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "direct"
		if nested {
			name = "nested"
		}
		t.Run(name, func(t *testing.T) {
			parent := NewPool(1)
			defer parent.StopAndWait()
			subpool := parent.NewSubpool(1)
			var intermediate Pool
			if nested {
				intermediate = subpool
				subpool = intermediate.NewSubpool(1)
			}

			started := make(chan struct{})
			release := make(chan struct{})
			aborted := subpool.Submit(func() {
				close(started)
				<-release
				runtime.Goexit()
			})
			<-started
			next := subpool.Submit(func() {})
			close(release)

			done := make(chan struct{})
			go func() {
				subpool.StopAndWait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("subpool did not recover after runtime.Goexit")
			}

			assert.Equal(t, ErrPoolStopped, aborted.Wait())
			assert.Equal(t, nil, next.Wait())
			assert.Equal(t, int64(0), subpool.RunningWorkers())
			assert.Equal(t, uint64(0), subpool.WaitingTasks())
			assert.Equal(t, uint64(1), subpool.FailedTasks())
			assert.Equal(t, uint64(1), subpool.SuccessfulTasks())
			if intermediate != nil {
				intermediate.StopAndWait()
			}
		})
	}
}

func TestSubpoolTrySubmitWaitsForParentQueueCapacity(t *testing.T) {
	parent := NewPool(1, WithQueueSize(1))
	parentTaskStarted := make(chan struct{})
	releaseParentTask := make(chan struct{})
	childTaskStarted := make(chan struct{})
	releaseChildTask := make(chan struct{})
	var releaseParentOnce, releaseChildOnce sync.Once
	releaseParent := func() { releaseParentOnce.Do(func() { close(releaseParentTask) }) }
	releaseChild := func() { releaseChildOnce.Do(func() { close(releaseChildTask) }) }
	defer func() {
		releaseParent()
		releaseChild()
		parent.StopAndWait()
	}()

	if err := parent.Go(func() {
		close(parentTaskStarted)
		<-releaseParentTask
	}); err != nil {
		t.Fatalf("start parent task: %v", err)
	}
	<-parentTaskStarted
	if err := parent.Go(func() {}); err != nil {
		t.Fatalf("fill parent queue: %v", err)
	}

	subpool := parent.NewSubpool(1, WithQueueSize(1), WithNonBlocking(true))
	first, accepted := subpool.TrySubmit(func() {
		close(childTaskStarted)
		<-releaseChildTask
	})
	if !accepted {
		t.Fatal("TrySubmit rejected work despite available subpool capacity")
	}
	second := subpool.Submit(func() {})
	releaseParent()
	select {
	case <-childTaskStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("subpool task did not run after the parent queue freed capacity")
	}
	if err := parent.Go(func() {}); err != nil {
		t.Fatalf("refill parent queue before subpool handoff: %v", err)
	}
	releaseChild()

	for _, task := range []Task{first, second} {
		select {
		case <-task.Done():
			if err := task.Wait(); err != nil {
				t.Fatalf("subpool task error = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("subpool task did not run after the parent queue freed capacity")
		}
	}

	subpool.StopAndWait()
	assert.Equal(t, uint64(2), subpool.SubmittedTasks())
	assert.Equal(t, uint64(2), subpool.CompletedTasks())
	assert.Equal(t, uint64(2), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.FailedTasks())
	assert.Equal(t, uint64(0), subpool.DroppedTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())
	assert.Equal(t, int64(0), subpool.RunningWorkers())

	parent.StopAndWait()
	assert.Equal(t, uint64(5), parent.SubmittedTasks())
	assert.Equal(t, uint64(5), parent.CompletedTasks())
	assert.Equal(t, uint64(5), parent.SuccessfulTasks())
	assert.Equal(t, uint64(0), parent.FailedTasks())
	assert.Equal(t, uint64(0), parent.DroppedTasks())
	assert.Equal(t, uint64(0), parent.WaitingTasks())
	assert.Equal(t, int64(0), parent.RunningWorkers())
}

func TestSubpoolPendingParentScheduleCancelsWithParentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	parent := NewPool(1, WithQueueSize(1), WithContext(ctx))
	parentTaskStarted := make(chan struct{})
	releaseParentTask := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseParentTask) }) }
	defer func() {
		release()
		cancel()
		parent.StopAndWait()
	}()

	if err := parent.Go(func() {
		close(parentTaskStarted)
		<-releaseParentTask
	}); err != nil {
		t.Fatalf("start parent task: %v", err)
	}
	<-parentTaskStarted
	if err := parent.Go(func() {}); err != nil {
		t.Fatalf("fill parent queue: %v", err)
	}

	subpool := parent.NewSubpool(1, WithQueueSize(3), WithNonBlocking(true))
	var executedCount atomic.Int64
	tasks := make([]Task, 3)
	var accepted bool
	tasks[0], accepted = subpool.TrySubmit(func() { executedCount.Add(1) })
	if !accepted {
		t.Fatal("TrySubmit rejected work despite available subpool capacity")
	}
	for i := 1; i < len(tasks); i++ {
		tasks[i] = subpool.Submit(func() { executedCount.Add(1) })
	}
	cancel()

	for _, task := range tasks {
		select {
		case <-task.Done():
			if err := task.Wait(); !errors.Is(err, context.Canceled) {
				t.Fatalf("task error = %v, want context cancellation", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("pending subpool task was not canceled with its parent")
		}
	}
	assert.Equal(t, int64(0), executedCount.Load())

	release()
	subpool.StopAndWait()
	parent.StopAndWait()

	assert.Equal(t, uint64(3), subpool.SubmittedTasks())
	assert.Equal(t, uint64(0), subpool.CompletedTasks())
	assert.Equal(t, uint64(0), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.FailedTasks())
	assert.Equal(t, uint64(3), subpool.CanceledTasks())
	assert.Equal(t, uint64(0), subpool.DroppedTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())
	assert.Equal(t, int64(0), subpool.RunningWorkers())

	assert.Equal(t, uint64(3), parent.SubmittedTasks())
	assert.Equal(t, uint64(2), parent.CompletedTasks())
	assert.Equal(t, uint64(2), parent.SuccessfulTasks())
	assert.Equal(t, uint64(0), parent.FailedTasks())
	assert.Equal(t, uint64(3), parent.CanceledTasks())
	assert.Equal(t, uint64(1), parent.DroppedTasks())
	assert.Equal(t, uint64(0), parent.WaitingTasks())
	assert.Equal(t, int64(0), parent.RunningWorkers())
}

func TestSubpoolStopCancelsPendingParentSchedule(t *testing.T) {
	parent := NewPool(1, WithQueueSize(1))
	parentTaskStarted := make(chan struct{})
	releaseParentTask := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseParentTask) }) }
	defer func() {
		release()
		parent.StopAndWait()
	}()

	if err := parent.Go(func() {
		close(parentTaskStarted)
		<-releaseParentTask
	}); err != nil {
		t.Fatalf("start parent task: %v", err)
	}
	<-parentTaskStarted
	if err := parent.Go(func() {}); err != nil {
		t.Fatalf("fill parent queue: %v", err)
	}

	subpool := parent.NewSubpool(1, WithQueueSize(1), WithNonBlocking(true))
	task, accepted := subpool.TrySubmit(func() {})
	if !accepted {
		t.Fatal("TrySubmit rejected work despite available subpool capacity")
	}

	stopped := make(chan struct{})
	go func() {
		subpool.StopAndWait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("subpool stop waited forever for a full parent queue")
	}
	if err := task.Wait(); !errors.Is(err, ErrPoolStopped) {
		t.Fatalf("task error = %v, want %v", err, ErrPoolStopped)
	}
}

func TestParentStopAndWaitDrainsPendingSubpoolSchedule(t *testing.T) {
	parent := NewPool(1, WithQueueSize(1))
	parentTaskStarted := make(chan struct{})
	releaseParentTask := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseParentTask) }) }
	defer func() {
		release()
		parent.StopAndWait()
	}()

	if err := parent.Go(func() {
		close(parentTaskStarted)
		<-releaseParentTask
	}); err != nil {
		t.Fatalf("start parent task: %v", err)
	}
	<-parentTaskStarted
	if err := parent.Go(func() {}); err != nil {
		t.Fatalf("fill parent queue: %v", err)
	}

	subpool := parent.NewSubpool(1, WithQueueSize(3), WithNonBlocking(true))
	var executedCount atomic.Int64
	tasks := make([]Task, 3)
	var accepted bool
	tasks[0], accepted = subpool.TrySubmit(func() { executedCount.Add(1) })
	if !accepted {
		t.Fatal("TrySubmit rejected work despite available subpool capacity")
	}
	for i := 1; i < len(tasks); i++ {
		tasks[i] = subpool.Submit(func() { executedCount.Add(1) })
	}

	stopped := make(chan struct{})
	go func() {
		parent.StopAndWait()
		close(stopped)
	}()
	for _, task := range tasks {
		select {
		case <-task.Done():
			if err := task.Wait(); !errors.Is(err, ErrPoolStopped) {
				t.Fatalf("task error = %v, want %v", err, ErrPoolStopped)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("pending subpool future did not complete when parent stopped")
		}
	}
	assert.Equal(t, int64(0), executedCount.Load())

	release()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("parent StopAndWait did not finish after releasing its active task")
	}
	subpool.StopAndWait()

	assert.Equal(t, uint64(3), subpool.SubmittedTasks())
	assert.Equal(t, uint64(0), subpool.CompletedTasks())
	assert.Equal(t, uint64(0), subpool.SuccessfulTasks())
	assert.Equal(t, uint64(0), subpool.FailedTasks())
	assert.Equal(t, uint64(3), subpool.CanceledTasks())
	assert.Equal(t, uint64(0), subpool.DroppedTasks())
	assert.Equal(t, uint64(0), subpool.WaitingTasks())
	assert.Equal(t, int64(0), subpool.RunningWorkers())

	assert.Equal(t, uint64(3), parent.SubmittedTasks())
	assert.Equal(t, uint64(2), parent.CompletedTasks())
	assert.Equal(t, uint64(2), parent.SuccessfulTasks())
	assert.Equal(t, uint64(0), parent.FailedTasks())
	assert.Equal(t, uint64(3), parent.CanceledTasks())
	assert.Equal(t, uint64(1), parent.DroppedTasks())
	assert.Equal(t, uint64(0), parent.WaitingTasks())
	assert.Equal(t, int64(0), parent.RunningWorkers())
}
