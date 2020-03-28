package pond

import (
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultIdleTimeout = 5 * time.Second
)

func defaultPanicHandler(panic interface{}) {
	fmt.Printf("Worker exits from a panic: %v\nStack trace: %s\n", panic, string(debug.Stack()))
}

func linearGrowthFn(workerCount, minWorkers, maxWorkers int) int {
	if workerCount < minWorkers {
		return minWorkers
	}
	if workerCount < maxWorkers {
		return 1
	}
	return 0
}

// Option represents an option that can be passed when building a worker pool to customize it
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

// New creates a worker pool with that can scale up to the given number of workers and capacity
func New(maxWorkers, maxCapacity int, options ...Option) *WorkerPool {

	pool := &WorkerPool{
		maxWorkers:      maxWorkers,
		maxCapacity:     maxCapacity,
		idleTimeout:     defaultIdleTimeout,
		tasks:           make(chan func(), maxCapacity),
		dispatchedTasks: make(chan func(), maxWorkers),
		purgerQuit:      make(chan struct{}),
		panicHandler:    defaultPanicHandler,
		growthFn:        linearGrowthFn,
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
// it will wait until the task can be enqueued.
func (p *WorkerPool) Submit(task func()) {
	if task == nil {
		return
	}

	// Submit the task to the task channel
	p.tasks <- task
}

// SubmitAndWait sends a task to this worker pool for execution and waits for it to complete
// before returning.
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

// Stop causes this pool to stop accepting tasks, without waiting for goroutines to exit
func (p *WorkerPool) Stop() {
	p.stop(false)
}

// StopAndWait causes this pool to stop accepting tasks, waiting for all tasks in the queue to complete
func (p *WorkerPool) StopAndWait() {
	p.stop(true)
}

// dispatch represents the work done by the dispatcher goroutine
func (p *WorkerPool) dispatch() {

	for task := range p.tasks {
		if task == nil {
			// Received the signal to exit gracefully
			break
		}

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

	// Send signal to stop all workers
	close(p.dispatchedTasks)

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
			return
		}
	}
}

func (p *WorkerPool) startWorkers() {

	count := p.growthFn(p.Running(), p.minWorkers, p.maxWorkers)

	if count == 0 {
		return
	}

	// Increment worker count
	atomic.AddInt32(&p.workerCount, int32(count))

	// Increment waiting group semaphore
	p.waitGroup.Add(count)

	//go func() {
	for i := 0; i < count; i++ {
		worker(p.dispatchedTasks, func() {

			// Decrement worker count
			atomic.AddInt32(&p.workerCount, -1)

			// Decrement waiting group semaphore
			p.waitGroup.Done()

		}, p.panicHandler)
	}
	//}()

}

// Stop causes this pool to stop accepting tasks, without waiting for goroutines to exit
func (p *WorkerPool) stop(wait bool) {

	p.stopOnce.Do(func() {
		if wait {
			// Make sure all queued tasks complete before stopping the dispatcher
			p.tasks <- nil

			// Close the tasks channel to prevent receiving new tasks
			close(p.tasks)

			// Wait for all goroutines to exit
			p.waitGroup.Wait()
		} else {
			// Close the tasks channel to prevent receiving new tasks
			close(p.tasks)
		}
	})
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

// Submit adds a task to this group and sends it to the worker pool to be executed.
func (g *TaskGroup) Submit(task func()) {
	g.waitGroup.Add(1)
	g.pool.Submit(func() {
		defer g.waitGroup.Done()
		task()
	})
}

// Wait waits until all the tasks in this group have completed. It returns
// a slice with all (non-nil) errors returned by tasks in this group.
func (g *TaskGroup) Wait() {

	// Wait for all tasks to complete
	g.waitGroup.Wait()
}
