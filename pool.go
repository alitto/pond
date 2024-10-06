package pond

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/dispatcher"
	"github.com/alitto/pond/v2/internal/future"
)

const DEFAULT_TASKS_CHAN_LENGTH = 2048

type MeteredPool interface {
	// Returns the number of worker goroutines that are currently active (executing a task) in the pool.
	RunningWorkers() int64

	// Returns the total number of tasks submitted to the pool since its creation.
	SubmittedTasks() uint64

	// Returns the number of tasks that are currently waiting in the pool's queue.
	WaitingTasks() uint64

	// Returns the number of tasks that have completed with an error.
	FailedTasks() uint64

	// Returns the number of tasks that have completed successfully.
	SuccessfulTasks() uint64

	// Returns the total number of tasks that have completed (either successfully or with an error).
	CompletedTasks() uint64
}

/**
 * Interface implemented by all worker pools in this library.
 * @param O The type of the output of the tasks submitted to the pool
 */
type AbstractPool interface {
	MeteredPool

	// Returns the context associated with this pool.
	Context() context.Context

	// Runs the task
	Go(task func())

	// Stops the pool and returns a future that can be used to wait for all tasks pending to complete.
	Stop() Async

	// Stops the pool and waits for all tasks to complete.
	StopAndWait()
}

type Pool interface {
	AbstractPool

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	Submit(task func()) Async

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	SubmitErr(task func() error) Async

	// Creates a new subpool with the specified maximum concurrency.
	Subpool(maxConcurrency int) Pool

	// Creates a new task group.
	Group() TaskGroup
}

// pool is an implementation of the Pool interface.
type pool struct {
	ctx                 context.Context
	maxConcurrency      int
	tasks               chan any
	tasksLen            int
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	dispatcher          *dispatcher.Dispatcher[any]
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
	// Subpool properties
	parent    AbstractPool
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

func (p *pool) Context() context.Context {
	return p.ctx
}

func (p *pool) RunningWorkers() int64 {
	return p.workerCount.Load()
}

func (p *pool) SubmittedTasks() uint64 {
	return p.dispatcher.WriteCount()
}

func (p *pool) WaitingTasks() uint64 {
	return p.dispatcher.Len()
}

func (p *pool) FailedTasks() uint64 {
	return p.failedTaskCount.Load()
}

func (p *pool) SuccessfulTasks() uint64 {
	return p.successfulTaskCount.Load()
}

func (p *pool) CompletedTasks() uint64 {
	return p.successfulTaskCount.Load() + p.failedTaskCount.Load()
}

func (p *pool) updateMetrics(err error) {
	if err != nil {
		p.failedTaskCount.Add(1)
	} else {
		p.successfulTaskCount.Add(1)
	}
}

func (p *pool) Go(task func()) {
	p.dispatcher.Write(task)
}

func (p *pool) Submit(task func()) Async {
	future, resolve := future.NewFuture[struct{}](p.Context())

	wrapped := wrapTask(task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *pool) SubmitErr(task func() error) Async {
	future, resolve := future.NewFuture[struct{}](p.Context())

	wrapped := wrapTaskErr(task, resolve)

	p.dispatcher.Write(wrapped)

	return future
}

func (p *pool) Stop() Async {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()

		if p.sem != nil {
			close(p.sem)
		}

		close(p.tasks)

		p.workerWaitGroup.Wait()
	})
}

// StopAndWait stops the pool and waits for all tasks to complete.
func (p *pool) StopAndWait() {
	p.Stop().Wait()
}

func (p *pool) Subpool(maxConcurrency int) Pool {
	return newPool(maxConcurrency, p.ctx, p)
}

func (p *pool) Group() TaskGroup {
	return NewTaskGroup(p)
}

func (p *pool) dispatch(incomingTasks []any) {

	var workerCount int

	// Submit tasks
	for _, task := range incomingTasks {

		workerCount = int(p.workerCount.Load())

		if workerCount < p.tasksLen {
			// If there are less workers than the size of the channel, start workers
			p.startWorker()
		}

		// Attempt to submit task without blocking
		select {
		case p.tasks <- task:
			// Task submitted
			continue
		default:
		}

		// There are no idle workers, create more
		if workerCount < p.maxConcurrency {
			// Launch a new worker
			p.startWorker()
		}

		// Block until task is submitted
		select {
		case p.tasks <- task:
			// Task submitted
		case <-p.ctx.Done():
			// Context cancelled, exit
			return
		}
	}
}

func (p *pool) subpoolDispatch(incomingTasks []any) {

	p.waitGroup.Add(len(incomingTasks))

	// Submit tasks
	for _, task := range incomingTasks {

		select {
		case <-p.Context().Done():
			// Context canceled, exit
			return
		case p.sem <- struct{}{}:
			// Acquired the semaphore, submit another task
		}

		subpoolTask := subpoolTask[any]{
			task:          task,
			sem:           p.sem,
			waitGroup:     &p.waitGroup,
			updateMetrics: p.updateMetrics,
		}

		p.parent.Go(subpoolTask.Run)
	}
}

func (p *pool) startWorker() {
	p.workerWaitGroup.Add(1)
	p.workerCount.Add(1)
	go p.worker()
}

func (p *pool) worker() {
	defer func() {
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
	}()

	for {
		// Prioritize context cancellation over task execution
		select {
		case <-p.ctx.Done():
			// Context cancelled, exit
			return
		default:
		}

		select {
		case <-p.ctx.Done():
			// Context cancelled, exit
			return
		case task, ok := <-p.tasks:
			if !ok || task == nil {
				// Channel closed or worker killed, exit
				return
			}

			// Execute task
			_, err := invokeTask[any](task)

			// Update metrics
			p.updateMetrics(err)

		default:
			// No tasks left, exit
			return
		}
	}
}

func newPool(maxConcurrency int, ctx context.Context, parent AbstractPool) *pool {

	if maxConcurrency <= 0 {
		panic("maxConcurrency must be greater than 0")
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	pool := &pool{
		ctx:            ctx,
		maxConcurrency: maxConcurrency,
		tasks:          make(chan any, tasksLen),
		tasksLen:       tasksLen,
		parent:         parent,
	}

	if parent != nil {
		// Subpool
		pool.sem = make(chan struct{}, maxConcurrency)
		pool.dispatcher = dispatcher.NewDispatcher(pool.ctx, pool.subpoolDispatch, tasksLen)
	} else {
		pool.dispatcher = dispatcher.NewDispatcher(pool.ctx, pool.dispatch, tasksLen)
	}

	return pool
}

// NewPool creates a new pool with the specified maximum concurrency and options.
func NewPool(maxConcurrency int) Pool {
	return newPool(maxConcurrency, context.Background(), nil)
}
