package pond

import (
	"fmt"
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

// linearGrowthFn is a function that determines how many workers to create when backpressure is detected
func linearGrowthFn(workerCount, minWorkers, maxWorkers int) int {
	if workerCount < minWorkers {
		return minWorkers
	}
	if workerCount < maxWorkers {
		return 1
	}
	return 0
}

// Option represents an option that can be passed when instantiating a worker pool to customize it
type Option func(*WorkerPool)

// IdleTimeout allows to change the idle timeout for a worker pool
func IdleTimeout(idleTimeout time.Duration) Option {
	return func(pool *WorkerPool) {
		pool.idleTimeout = idleTimeout
	}
}

// PanicHandler allows to change the panic handler function for a worker pool
func PanicHandler(panicHandler func(interface{})) Option {
	return func(pool *WorkerPool) {
		pool.panicHandler = panicHandler
	}
}

// MinWorkers allows to change the minimum number of workers of a worker pool
func MinWorkers(minWorkers int) Option {
	return func(pool *WorkerPool) {
		pool.minWorkers = minWorkers
	}
}

// WorkerPool models a pool of workers
type WorkerPool struct {
	minWorkers      int
	maxWorkers      int
	maxCapacity     int
	idleTimeout     time.Duration
	workerCount     int32
	tasks           chan func()
	dispatchedTasks chan func()
	purgerQuit      chan struct{}
	stopOnce        sync.Once
	waitGroup       sync.WaitGroup
	panicHandler    func(interface{})
	growthFn        func(int, int, int) int
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
		purgerQuit:   make(chan struct{}),
		panicHandler: defaultPanicHandler,
		growthFn:     linearGrowthFn,
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

	// Create channels
	pool.tasks = make(chan func(), pool.maxCapacity)
	pool.dispatchedTasks = make(chan func(), pool.maxWorkers)

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
		pool.startWorkers()
	}

	return pool
}

// Running returns the number of running workers
func (p *WorkerPool) Running() int {
	return int(atomic.LoadInt32(&p.workerCount))
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
		// Close the tasks channel to prevent receiving new tasks
		close(p.tasks)
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

	batchSize := p.maxWorkers
	batch := make([]func(), 0)

	for task := range p.tasks {
		batch = append(batch, task)

		// Read up to batchSize - 1 tasks without blocking
	BulkReceive:
		for i := 0; i < batchSize; i++ {
			select {
			case t := <-p.tasks:
				batch = append(batch, t)
			default:
				break BulkReceive
			}
		}

		for _, task := range batch {
			select {
			// Attempt to submit the task to a worker without blocking
			case p.dispatchedTasks <- task:
				if p.Running() == 0 {
					p.startWorkers()
				}
			default:
				// Create a new worker if we haven't reached the limit yet
				if p.Running() < p.maxWorkers {
					p.startWorkers()
				}

				// Block until a worker accepts this task
				p.dispatchedTasks <- task
			}
		}

		// Clear batch slice
		batch = nil
	}

	// Send signal to stop the purger
	close(p.purgerQuit)
}

// purge represents the work done by the purger goroutine
func (p *WorkerPool) purge() {
	ticker := time.NewTicker(p.idleTimeout)
	defer ticker.Stop()

	for {
		select {
		// Timed out waiting for any activity to happen, attempt to stop an idle worker
		case <-ticker.C:
			if p.Running() > p.minWorkers {
				select {
				case p.dispatchedTasks <- nil:
				default:
					// If dispatchedTasks channel is full, no need to kill the worker
				}
			}

		// Received the signal to exit
		case <-p.purgerQuit:

			// Send signal to stop all workers
			close(p.dispatchedTasks)

			return
		}
	}
}

// startWorkers launches worker goroutines according to the growth function
func (p *WorkerPool) startWorkers() {

	count := p.growthFn(p.Running(), p.minWorkers, p.maxWorkers)

	// Increment worker count
	atomic.AddInt32(&p.workerCount, int32(count))

	// Increment waiting group semaphore
	p.waitGroup.Add(count)

	for i := 0; i < count; i++ {
		worker(p.dispatchedTasks, func() {

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
func worker(tasks chan func(), exitHandler func(), panicHandler func(interface{})) {

	go func() {
		defer func() {
			if panic := recover(); panic != nil {
				// Handle panic
				panicHandler(panic)

				// Restart goroutine
				worker(tasks, exitHandler, panicHandler)
			} else {
				// Handle exit
				exitHandler()
			}
		}()

		for task := range tasks {
			if task == nil {
				// We have received a signal to quit
				return
			}

			// We have received a task, execute it
			task()
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
