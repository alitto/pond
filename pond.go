package pond

import (
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultIdleTimeout defines the default idle timeout to use when not explicitly specified
	// via the IdleTimeout() option
	defaultIdleTimeout = 5 * time.Second
)

// defaultPanicHandler is the default panic handler
func defaultPanicHandler(panic interface{}) {
	fmt.Printf("Worker exits from a panic: %v\nStack trace: %s\n", panic, string(debug.Stack()))
}

// ResizingStrategy represents a pool resizing strategy
type ResizingStrategy interface {
	Resize(runningWorkers, idleWorkers, minWorkers, maxWorkers, incomingTasks, completedTasks int, delta time.Duration) int
}

// Option represents an option that can be passed when instantiating a worker pool to customize it
type Option func(*WorkerPool)

// IdleTimeout allows to change the idle timeout for a worker pool
func IdleTimeout(idleTimeout time.Duration) Option {
	return func(pool *WorkerPool) {
		pool.idleTimeout = idleTimeout
	}
}

// MinWorkers allows to change the minimum number of workers of a worker pool
func MinWorkers(minWorkers int) Option {
	return func(pool *WorkerPool) {
		pool.minWorkers = minWorkers
	}
}

// Strategy allows to change the strategy used to resize the pool
func Strategy(strategy ResizingStrategy) Option {
	return func(pool *WorkerPool) {
		pool.strategy = strategy
	}
}

// PanicHandler allows to change the panic handler function for a worker pool
func PanicHandler(panicHandler func(interface{})) Option {
	return func(pool *WorkerPool) {
		pool.panicHandler = panicHandler
	}
}

// WorkerPool models a pool of workers
type WorkerPool struct {
	// Configurable settings
	maxWorkers   int
	maxCapacity  int
	minWorkers   int
	idleTimeout  time.Duration
	strategy     ResizingStrategy
	panicHandler func(interface{})
	// Atomic counters
	workerCount        int32
	idleWorkerCount    int32
	completedTaskCount uint64
	// Private properties
	tasks           chan func()
	dispatchedTasks chan func()
	purgerQuit      chan struct{}
	stopOnce        sync.Once
	waitGroup       sync.WaitGroup
	// Debug information
	debug          bool
	maxWorkerCount int
}

// New creates a worker pool with that can scale up to the given maximum number of workers (maxWorkers).
// The maxCapacity parameter determines the number of tasks that can be submitted to this pool without blocking,
// because it defines the size of the buffered channel used to receive tasks.
// The options parameter can take a list of functions to customize configuration values on this worker pool.
func New(maxWorkers, maxCapacity int, options ...Option) *WorkerPool {

	// Instantiate the pool
	pool := &WorkerPool{
		maxWorkers:   maxWorkers,
		maxCapacity:  maxCapacity,
		idleTimeout:  defaultIdleTimeout,
		strategy:     Balanced,
		panicHandler: defaultPanicHandler,
	}

	// Apply all options
	for _, opt := range options {
		opt(pool)
	}

	// Make sure options are consistent
	if pool.maxWorkers <= 0 {
		pool.maxWorkers = 1
	}
	if pool.minWorkers > pool.maxWorkers {
		pool.minWorkers = pool.maxWorkers
	}
	if pool.maxCapacity < 0 {
		pool.maxCapacity = 0
	}
	if pool.idleTimeout < 0 {
		pool.idleTimeout = defaultIdleTimeout
	}

	// Create internal channels
	pool.tasks = make(chan func(), pool.maxCapacity)
	pool.dispatchedTasks = make(chan func(), pool.maxWorkers)
	pool.purgerQuit = make(chan struct{})

	// Start dispatcher goroutine
	pool.waitGroup.Add(1)
	go func() {
		defer pool.waitGroup.Done()

		pool.dispatch()
	}()

	// Start purger goroutine
	pool.waitGroup.Add(1)
	go func() {
		defer pool.waitGroup.Done()

		pool.purge()
	}()

	// Start minWorkers workers
	if pool.minWorkers > 0 {
		pool.startWorkers(pool.minWorkers, nil)
	}

	return pool
}

// Running returns the number of running workers
func (p *WorkerPool) Running() int {
	return int(atomic.LoadInt32(&p.workerCount))
}

// Idle returns the number of idle workers
func (p *WorkerPool) Idle() int {
	return int(atomic.LoadInt32(&p.idleWorkerCount))
}

// Submit sends a task to this worker pool for execution. If the queue is full,
// it will wait until the task can be enqueued
func (p *WorkerPool) Submit(task func()) {
	if task == nil {
		return
	}

	// Submit the task to the task channel
	p.tasks <- task
}

// SubmitAndWait sends a task to this worker pool for execution and waits for it to complete
// before returning
func (p *WorkerPool) SubmitAndWait(task func()) {
	if task == nil {
		return
	}

	done := make(chan struct{})
	p.Submit(func() {
		defer close(done)
		task()
	})
	<-done
}

// SubmitBefore attempts to send a task for execution to this worker pool but aborts it
// if the task did not start before the given deadline
func (p *WorkerPool) SubmitBefore(task func(), deadline time.Duration) {
	if task == nil {
		return
	}

	timer := time.NewTimer(deadline)
	p.Submit(func() {
		select {
		case <-timer.C:
			// Deadline was reached, abort the task
		default:
			// Deadline not reached, execute the task
			defer timer.Stop()

			task()
		}
	})
}

// Stop causes this pool to stop accepting tasks, without waiting for goroutines to exit
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		// Send signal to stop the purger
		close(p.purgerQuit)
	})
}

// StopAndWait causes this pool to stop accepting tasks, waiting for all tasks in the queue to complete
func (p *WorkerPool) StopAndWait() {
	p.Stop()

	// Wait for all goroutines to exit
	p.waitGroup.Wait()
}

// dispatch represents the work done by the dispatcher goroutine
func (p *WorkerPool) dispatch() {

	batch := make([]func(), 0)
	batchSize := int(math.Max(float64(p.minWorkers), 1000))
	var lastCompletedTasks uint64 = 0
	var lastCycle time.Time = time.Now()

	for task := range p.tasks {

		idleCount := p.Idle()
		dispatchedImmediately := 0

		// Dispatch up to idleCount tasks without blocking
		nextTask := task
	ImmediateDispatch:
		for i := 0; i < idleCount; i++ {

			// Attempt to dispatch
			select {
			case p.dispatchedTasks <- nextTask:
				dispatchedImmediately++
			default:
				break ImmediateDispatch
			}

			// Attempt to receive another task
			select {
			case t, ok := <-p.tasks:
				if !ok {
					// Nothing to dispatch
					nextTask = nil
					break ImmediateDispatch
				}
				nextTask = t
			default:
				nextTask = nil
				break ImmediateDispatch
			}
		}
		if nextTask == nil {
			continue
		}

		// Start batching tasks
		batch = append(batch, nextTask)

		// Read up to batchSize tasks without blocking
	BulkReceive:
		for i := 0; i < batchSize-1; i++ {
			select {
			case t, ok := <-p.tasks:
				if !ok {
					break BulkReceive
				}
				if t != nil {
					batch = append(batch, t)
				}
			default:
				break BulkReceive
			}
		}

		// Resize the pool
		now := time.Now()
		delta := now.Sub(lastCycle)
		workload := len(batch)
		runningCount := p.Running()
		lastCycle = now
		currentCompletedTasks := atomic.LoadUint64(&p.completedTaskCount)
		completedTasks := int(currentCompletedTasks - lastCompletedTasks)
		if completedTasks < 0 {
			completedTasks = 0
		}
		lastCompletedTasks = currentCompletedTasks
		targetDelta := p.calculatePoolSizeDelta(runningCount, idleCount, workload+dispatchedImmediately, completedTasks, delta)

		// Start up to targetDelta workers
		dispatched := 0
		if targetDelta > 0 {
			p.startWorkers(targetDelta, batch)
			dispatched = workload
			if targetDelta < workload {
				dispatched = targetDelta
			}
		} else if targetDelta < 0 {
			// Kill targetDelta workers
			for i := 0; i < -targetDelta; i++ {
				p.dispatchedTasks <- nil
			}
		}

		dispatchedBlocking := 0

		if workload > dispatched {
			for _, task := range batch[dispatched:] {
				// Attempt to dispatch the task without blocking
				select {
				case p.dispatchedTasks <- task:
				default:
					// Block until a worker accepts this task
					p.dispatchedTasks <- task
					dispatchedBlocking++
				}
			}
		}

		// Adjust batch size
		if dispatchedBlocking > 0 {
			if batchSize > 1 {
				batchSize = 1
			}
		} else {
			maxBatchSize := runningCount + targetDelta
			batchSize = batchSize * 2
			if batchSize > maxBatchSize {
				batchSize = maxBatchSize
			}
		}

		// Clear batch slice
		batch = nil
	}

	// Send signal to stop all workers
	close(p.dispatchedTasks)
}

// purge represents the work done by the purger goroutine
func (p *WorkerPool) purge() {
	ticker := time.NewTicker(p.idleTimeout)
	defer ticker.Stop()

	for {
		select {
		// Timed out waiting for any activity to happen, attempt to resize the pool
		case <-ticker.C:
			if p.Idle() > 0 {
				select {
				case p.tasks <- nil:
				default:
					// If tasks channel is full, there's no need to resize the pool
				}
			}

		// Received the signal to exit
		case <-p.purgerQuit:

			// Close the tasks channel to prevent receiving new tasks
			close(p.tasks)

			return
		}
	}
}

// calculatePoolSizeDelta calculates what's the delta to reach the ideal pool size based on the current size and workload
func (p *WorkerPool) calculatePoolSizeDelta(runningWorkers, idleWorkers,
	incomingTasks, completedTasks int, duration time.Duration) int {

	delta := p.strategy.Resize(runningWorkers, idleWorkers, p.minWorkers, p.maxWorkers,
		incomingTasks, completedTasks, duration)

	targetSize := runningWorkers + delta

	// Cannot go below minWorkers
	if targetSize < p.minWorkers {
		targetSize = p.minWorkers
	}
	// Cannot go above maxWorkers
	if targetSize > p.maxWorkers {
		targetSize = p.maxWorkers
	}

	if p.debug {
		// Print debugging information
		durationSecs := duration.Seconds()
		inputRate := float64(incomingTasks) / durationSecs
		outputRate := float64(completedTasks) / durationSecs
		message := fmt.Sprintf("%d\t%d\t%d\t%d\t\"%f\"\t\"%f\"\t%d\t\"%f\"\n",
			runningWorkers, idleWorkers, incomingTasks, completedTasks,
			inputRate, outputRate,
			delta, durationSecs)
		fmt.Printf(message)
	}

	return targetSize - runningWorkers
}

// startWorkers creates new worker goroutines to run the given tasks
func (p *WorkerPool) startWorkers(count int, firstTasks []func()) {

	// Increment worker count
	workerCount := atomic.AddInt32(&p.workerCount, int32(count))

	// Collect debug information
	if p.debug && int(workerCount) > p.maxWorkerCount {
		p.maxWorkerCount = int(workerCount)
	}

	// Increment waiting group semaphore
	p.waitGroup.Add(count)

	// Launch workers
	for i := 0; i < count; i++ {
		var firstTask func() = nil
		if i < len(firstTasks) {
			firstTask = firstTasks[i]
		}
		worker(firstTask, p.dispatchedTasks, &p.idleWorkerCount, &p.completedTaskCount, func() {

			// Decrement worker count
			atomic.AddInt32(&p.workerCount, -1)

			// Decrement waiting group semaphore
			p.waitGroup.Done()

		}, p.panicHandler)
	}
}

// Group creates a new task group
func (p *WorkerPool) Group() *TaskGroup {
	return &TaskGroup{
		pool: p,
	}
}

// worker launches a worker goroutine
func worker(firstTask func(), tasks chan func(), idleWorkerCount *int32, completedTaskCount *uint64, exitHandler func(), panicHandler func(interface{})) {

	go func() {
		defer func() {
			if panic := recover(); panic != nil {
				// Handle panic
				panicHandler(panic)

				// Restart goroutine
				worker(nil, tasks, idleWorkerCount, completedTaskCount, exitHandler, panicHandler)
			} else {
				// Handle exit
				exitHandler()
			}
		}()

		// We have received a task, execute it
		func() {
			// Increment idle count
			defer atomic.AddInt32(idleWorkerCount, 1)
			if firstTask != nil {
				// Increment completed task count
				defer atomic.AddUint64(completedTaskCount, 1)

				firstTask()
			}
		}()

		for task := range tasks {
			if task == nil {
				// We have received a signal to quit
				return
			}

			// Decrement idle count
			atomic.AddInt32(idleWorkerCount, -1)

			// We have received a task, execute it
			func() {
				// Increment idle count
				defer atomic.AddInt32(idleWorkerCount, 1)

				// Increment completed task count
				defer atomic.AddUint64(completedTaskCount, 1)

				task()
			}()
		}
	}()
}

// TaskGroup represents a group of related tasks
type TaskGroup struct {
	pool      *WorkerPool
	waitGroup sync.WaitGroup
}

// Submit adds a task to this group and sends it to the worker pool to be executed
func (g *TaskGroup) Submit(task func()) {
	g.waitGroup.Add(1)
	g.pool.Submit(func() {
		defer g.waitGroup.Done()
		task()
	})
}

// Wait waits until all the tasks in this group have completed
func (g *TaskGroup) Wait() {

	// Wait for all tasks to complete
	g.waitGroup.Wait()
}
