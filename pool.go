package pond

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/dispatcher"
	"github.com/alitto/pond/v2/internal/future"
)

const DEFAULT_TASKS_CHAN_LENGTH = 2048

var ErrPoolStopped = errors.New("pool stopped")

var poolStoppedFuture = func() Task {
	future, resolve := future.NewFuture(context.Background())
	resolve(ErrPoolStopped)
	return future
}()

// basePool is the base interface for all pool types.
type basePool interface {
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

	// Returns the maximum concurrency of the pool.
	MaxConcurrency() int

	// Returns the context associated with this pool.
	Context() context.Context

	// Stops the pool and returns a future that can be used to wait for all tasks pending to complete.
	Stop() Task

	// Stops the pool and waits for all tasks to complete.
	StopAndWait()

	// Returns true if the pool has been stopped or its context has been cancelled.
	Stopped() bool
}

// Represents a pool of goroutines that can execute tasks concurrently.
type Pool interface {
	basePool

	// Submits a task to the pool without waiting for it to complete.
	Go(task func()) error

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	Submit(task func()) Task

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	SubmitErr(task func() error) Task

	// Creates a new subpool with the specified maximum concurrency.
	NewSubpool(maxConcurrency int) Pool

	// Creates a new task group.
	NewGroup() TaskGroup
}

// pool is an implementation of the Pool interface.
type pool struct {
	ctx                 context.Context
	cancel              context.CancelCauseFunc
	maxConcurrency      int
	tasks               chan any
	tasksLen            int
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	dispatcher          *dispatcher.Dispatcher[any]
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
}

func (p *pool) Context() context.Context {
	return p.ctx
}

func (p *pool) Stopped() bool {
	return p.ctx.Err() != nil
}

func (p *pool) MaxConcurrency() int {
	return p.maxConcurrency
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

func (p *pool) Go(task func()) error {
	if err := p.dispatcher.Write(task); err != nil {
		return ErrPoolStopped
	}

	return nil
}

func (p *pool) Submit(task func()) Task {
	return p.submit(task)
}

func (p *pool) SubmitErr(task func() error) Task {
	return p.submit(task)
}

func (p *pool) submit(task any) Task {
	future, resolve := future.NewFuture(p.Context())

	wrapped := wrapTask[struct{}, func(error)](task, resolve)

	if err := p.dispatcher.Write(wrapped); err != nil {
		return poolStoppedFuture
	}

	return future
}

func (p *pool) Stop() Task {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		close(p.tasks)

		p.workerWaitGroup.Wait()

		p.cancel(ErrPoolStopped)
	})
}

func (p *pool) StopAndWait() {
	p.Stop().Wait()
}

func (p *pool) NewSubpool(maxConcurrency int) Pool {
	return newSubpool(maxConcurrency, p.ctx, p)
}

func (p *pool) NewGroup() TaskGroup {
	return newTaskGroup(p)
}

func (p *pool) dispatch(incomingTasks []any) {
	// Submit tasks
	for _, task := range incomingTasks {
		p.dispatchTask(task)
	}
}

func (p *pool) dispatchTask(task any) {
	workerCount := int(p.workerCount.Load())

	// Attempt to submit task without blocking
	select {
	case p.tasks <- task:
		// Task submitted

		// If we could submit the task without blocking, it means one of two things:
		// 1. There are idle workers (all workers are busy)
		// 2. There are no workers
		// In either case, we should launch a new worker if the number of workers is less than the maximum concurrency
		if workerCount < p.maxConcurrency {
			// Launch a new worker
			p.startWorker()
		}
		return
	default:
	}

	// Task queue is full, launch a new worker if the number of workers is less than the maximum concurrency
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

func newPool(maxConcurrency int, options ...Option) *pool {

	if maxConcurrency == 0 {
		maxConcurrency = math.MaxInt
	}

	if maxConcurrency <= 0 {
		panic(errors.New("maxConcurrency must be greater than 0"))
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	pool := &pool{
		ctx:            context.Background(),
		maxConcurrency: maxConcurrency,
		tasks:          make(chan any, tasksLen),
		tasksLen:       tasksLen,
	}

	for _, option := range options {
		option(pool)
	}

	pool.ctx, pool.cancel = context.WithCancelCause(pool.ctx)

	pool.dispatcher = dispatcher.NewDispatcher(pool.ctx, pool.dispatch, tasksLen)

	return pool
}

// NewPool creates a new pool with the specified maximum concurrency and options.
func NewPool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, options...)
}
