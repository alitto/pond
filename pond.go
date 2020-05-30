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
	tasks                    chan func()
	dispatchedTasks          chan func()
	stopOnce                 sync.Once
	waitGroup                sync.WaitGroup
	lastResizeTime           time.Time
	lastResizeCompletedTasks uint64
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
		strategy:     Balanced(),
		panicHandler: defaultPanicHandler,
		debug:        false,
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

	// Start dispatcher goroutine
	pool.waitGroup.Add(1)
	go func() {
		defer pool.waitGroup.Done()

		pool.dispatch()
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

	// Declare vars
	var (
		maxBatchSize                   = 1000
		batch                          = make([]func(), maxBatchSize)
		batchSize                      = int(math.Max(float64(p.minWorkers), 100))
		idleWorkers                    = 0
		dispatchedToIdleWorkers        = 0
		dispatchedToNewWorkers         = 0
		dispatchedBlocking             = 0
		nextTask                func() = nil
	)

	idleTimer := time.NewTimer(p.idleTimeout)
	defer idleTimer.Stop()

	// Start dispatching cycle
DispatchCycle:
	for {
		// Reset idle timer
		idleTimer.Reset(p.idleTimeout)

		select {
		// Receive a task
		case task, ok := <-p.tasks:
			if !ok {
				// Received the signal to exit
				break DispatchCycle
			}

			idleWorkers = p.Idle()

			// Dispatch tasks to idle workers
			nextTask, dispatchedToIdleWorkers = p.dispatchToIdleWorkers(task, idleWorkers)
			if nextTask == nil {
				continue DispatchCycle
			}

			// Read up to batchSize tasks without blocking
			p.receiveBatch(nextTask, &batch, batchSize)

			// Resize the pool
			dispatchedToNewWorkers = p.resizePool(batch, dispatchedToIdleWorkers)

			dispatchedBlocking = 0
			if len(batch) > dispatchedToNewWorkers {
				for _, task := range batch[dispatchedToNewWorkers:] {
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
				batchSize = batchSize * 2
				if batchSize > maxBatchSize {
					batchSize = maxBatchSize
				}
			}
		// Timed out waiting for any activity to happen, attempt to resize the pool
		case <-idleTimer.C:
			p.resizePool(batch[:0], 0)
		}
	}

	// Send signal to stop all workers
	close(p.dispatchedTasks)

	if p.debug {
		fmt.Printf("Max workers: %d", p.maxWorkerCount)
	}
}

func (p *WorkerPool) dispatchToIdleWorkers(task func(), limit int) (nextTask func(), dispatched int) {

	// Dispatch up to limit tasks without blocking
	nextTask = task
	for i := 0; i < limit; i++ {

		// Attempt to dispatch without blocking
		select {
		case p.dispatchedTasks <- nextTask:
			nextTask = nil
			dispatched++
		default:
			// Could not dispatch, return the task
			return
		}

		// Attempt to receive another task
		select {
		case t, ok := <-p.tasks:
			if !ok {
				// Nothing else to dispatch
				nextTask = nil
				return
			}
			nextTask = t
		default:
			nextTask = nil
			return
		}
	}

	return
}

func (p *WorkerPool) receiveBatch(task func(), batch *[]func(), batchSize int) {

	// Reset batch slice
	*batch = (*batch)[:0]
	*batch = append(*batch, task)

	// Read up to batchSize tasks without blocking
	for i := 0; i < batchSize-1; i++ {
		select {
		case t, ok := <-p.tasks:
			if !ok {
				return
			}
			if t != nil {
				*batch = append(*batch, t)
			}
		default:
			return
		}
	}
}

func (p *WorkerPool) resizePool(batch []func(), dispatchedToIdleWorkers int) int {

	// Time to resize the pool
	now := time.Now()
	workload := len(batch)
	currentCompletedTasks := atomic.LoadUint64(&p.completedTaskCount)
	completedTasksDelta := int(currentCompletedTasks - p.lastResizeCompletedTasks)
	if completedTasksDelta < 0 {
		completedTasksDelta = 0
	}
	duration := 0 * time.Millisecond
	if !p.lastResizeTime.IsZero() {
		duration = now.Sub(p.lastResizeTime)
	}
	poolSizeDelta := p.calculatePoolSizeDelta(p.Running(), p.Idle(),
		workload+dispatchedToIdleWorkers, completedTasksDelta, duration)

	// Capture values for next resize cycle
	p.lastResizeTime = now
	p.lastResizeCompletedTasks = currentCompletedTasks

	// Start up to poolSizeDelta workers
	dispatched := 0
	if poolSizeDelta > 0 {
		p.startWorkers(poolSizeDelta, batch)
		dispatched = workload
		if poolSizeDelta < workload {
			dispatched = poolSizeDelta
		}
	} else if poolSizeDelta < 0 {
		// Kill poolSizeDelta workers
		for i := 0; i < -poolSizeDelta; i++ {
			p.dispatchedTasks <- nil
		}
	}

	return dispatched
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
	var firstTask func()
	for i := 0; i < count; i++ {
		firstTask = nil
		if i < len(firstTasks) {
			firstTask = firstTasks[i]
		}
		go worker(firstTask, p.dispatchedTasks, &p.idleWorkerCount, &p.completedTaskCount, p.decrementWorkers, p.panicHandler)
	}
}

func (p *WorkerPool) decrementWorkers() {

	// Decrement worker count
	atomic.AddInt32(&p.workerCount, -1)

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
func worker(firstTask func(), tasks chan func(), idleWorkerCount *int32, completedTaskCount *uint64, exitHandler func(), panicHandler func(interface{})) {

	defer func() {
		if panic := recover(); panic != nil {
			// Handle panic
			panicHandler(panic)

			// Restart goroutine
			go worker(nil, tasks, idleWorkerCount, completedTaskCount, exitHandler, panicHandler)
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
