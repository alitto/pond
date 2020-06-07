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

// ResizingStrategy represents a pool resizing strategy
type ResizingStrategy interface {
	Resize(runningWorkers, minWorkers, maxWorkers int) bool
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
	workerCount     int32
	idleWorkerCount int32
	// Private properties
	tasks      chan func()
	purgerQuit chan struct{}
	stopOnce   sync.Once
	waitGroup  sync.WaitGroup
	mutex      sync.Mutex
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
		strategy:     Eager(),
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
	pool.purgerQuit = make(chan struct{})

	// Start purger goroutine
	pool.waitGroup.Add(1)
	go func() {
		defer pool.waitGroup.Done()

		pool.purge()
	}()

	// Start minWorkers workers
	if pool.minWorkers > 0 {
		for i := 0; i < pool.minWorkers; i++ {
			pool.maybeStartWorker(nil)
		}
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
// it will wait until the task is dispatched to a worker goroutine.
func (p *WorkerPool) Submit(task func()) {
	p.submit(task, true)
}

// TrySubmit attempts to send a task to this worker pool for execution. If the queue is full,
// it will not wait for a worker to become idle. It returns true if it was able to dispatch
// the task and false otherwise.
func (p *WorkerPool) TrySubmit(task func()) bool {
	return p.submit(task, false)
}

func (p *WorkerPool) submit(task func(), waitForIdle bool) bool {
	if task == nil {
		return false
	}

	runningWorkerCount := p.Running()

	// Attempt to dispatch to an idle worker without blocking
	if runningWorkerCount > 0 && p.Idle() > 0 {
		select {
		case p.tasks <- task:
			return true
		default:
			// No idle worker available, continue
		}
	}

	maxWorkersReached := runningWorkerCount >= p.maxWorkers

	// Exit if we have reached the max. number of workers and can't wait for an idle worker
	if maxWorkersReached && !waitForIdle {
		return false
	}

	// Start a worker as long as we haven't reached the limit
	if ok := p.maybeStartWorker(task); ok {
		return true
	}

	// Submit the task to the tasks channel and wait for it to be picked up by a worker
	p.tasks <- task
	return true
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
		// Send the signal to stop the purger goroutine
		close(p.purgerQuit)
	})
}

// StopAndWait causes this pool to stop accepting tasks, waiting for all tasks in the queue to complete
func (p *WorkerPool) StopAndWait() {
	p.Stop()

	// Wait for all goroutines to exit
	p.waitGroup.Wait()
}

// purge represents the work done by the purger goroutine
func (p *WorkerPool) purge() {

	idleTicker := time.NewTicker(p.idleTimeout)
	defer idleTicker.Stop()

Purge:
	for {
		select {
		// Timed out waiting for any activity to happen, attempt to kill an idle worker
		case <-idleTicker.C:
			if p.Idle() > 0 && p.Running() > p.minWorkers {
				p.tasks <- nil
			}
		case <-p.purgerQuit:
			break Purge
		}
	}

	// Send signal to stop all workers
	close(p.tasks)

}

// startWorkers creates new worker goroutines to run the given tasks
func (p *WorkerPool) maybeStartWorker(firstTask func()) bool {

	// Attempt to increment worker count
	if ok := p.incrementWorkerCount(); !ok {
		return false
	}

	// Launch worker
	go worker(firstTask, p.tasks, &p.idleWorkerCount, p.decrementWorkerCount, p.panicHandler)

	return true
}

func (p *WorkerPool) incrementWorkerCount() bool {

	// Attempt to increment worker count
	p.mutex.Lock()
	runningWorkerCount := p.Running()
	// Execute the resizing strategy to determine if we can create more workers
	if !p.strategy.Resize(runningWorkerCount, p.minWorkers, p.maxWorkers) || runningWorkerCount >= p.maxWorkers {
		p.mutex.Unlock()
		return false
	}
	atomic.AddInt32(&p.workerCount, 1)
	p.mutex.Unlock()

	// Increment waiting group semaphore
	p.waitGroup.Add(1)

	return true
}

func (p *WorkerPool) decrementWorkerCount() {

	// Decrement worker count
	p.mutex.Lock()
	atomic.AddInt32(&p.workerCount, -1)
	p.mutex.Unlock()

	// Decrement waiting group semaphore
	p.waitGroup.Done()
}

// Group creates a new task group
func (p *WorkerPool) Group() *TaskGroup {
	return &TaskGroup{
		pool: p,
	}
}

// worker launches a worker goroutine
func worker(firstTask func(), tasks chan func(), idleWorkerCount *int32, exitHandler func(), panicHandler func(interface{})) {

	defer func() {

		if panic := recover(); panic != nil {
			// Handle panic
			panicHandler(panic)

			// Restart goroutine
			go worker(nil, tasks, idleWorkerCount, exitHandler, panicHandler)
		} else {
			// Decrement idle count
			atomic.AddInt32(idleWorkerCount, -1)

			// Handle normal exit
			exitHandler()
		}
	}()

	// We have received a task, execute it
	if firstTask != nil {
		firstTask()
	}
	// Increment idle count
	atomic.AddInt32(idleWorkerCount, 1)

	for task := range tasks {
		if task == nil {
			// We have received a signal to quit
			return
		}

		// Decrement idle count
		atomic.AddInt32(idleWorkerCount, -1)

		// We have received a task, execute it
		task()

		// Increment idle count
		atomic.AddInt32(idleWorkerCount, 1)
	}
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
