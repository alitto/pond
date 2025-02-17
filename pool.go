package pond

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/v2/internal/future"
	"github.com/alitto/pond/v2/internal/linkedbuffer"
)

const (
	DefaultQueueSize        = 0
	DefaultNonBlocking      = false
	LinkedBufferInitialSize = 1024
	LinkedBufferMaxCapacity = 100 * 1024
)

var (
	ErrQueueFull   = errors.New("queue is full")
	ErrQueueEmpty  = errors.New("queue is empty")
	ErrPoolStopped = errors.New("pool stopped")

	poolStoppedFuture = func() Task {
		future, resolve := future.NewFuture(context.Background())
		resolve(ErrPoolStopped)
		return future
	}()
)

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

	// Returns the size of the task queue.
	QueueSize() int

	// Returns true if the pool is non-blocking, meaning that it will not block when the task queue is full.
	// In a non-blocking pool, tasks that cannot be submitted to the queue will be dropped.
	// By default, pools are blocking, meaning that they will block when the task queue is full.
	NonBlocking() bool

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

	// Creates a new subpool with the specified maximum concurrency and options.
	NewSubpool(maxConcurrency int, options ...Option) Pool

	// Creates a new task group.
	NewGroup() TaskGroup

	// Creates a new task group with the specified context.
	NewGroupContext(ctx context.Context) TaskGroup
}

type pool struct {
	mutex               sync.Mutex
	parent              *pool
	ctx                 context.Context
	cancel              context.CancelCauseFunc
	nonBlocking         bool
	maxConcurrency      int
	closed              atomic.Bool
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	submitWaiters       chan struct{}
	queueSize           int
	tasks               *linkedbuffer.LinkedBuffer[any]
	submittedTaskCount  atomic.Uint64
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
}

func (p *pool) Context() context.Context {
	return p.ctx
}

func (p *pool) Stopped() bool {
	return p.closed.Load() || p.ctx.Err() != nil
}

func (p *pool) MaxConcurrency() int {
	return p.maxConcurrency
}

func (p *pool) QueueSize() int {
	return p.queueSize
}

func (p *pool) NonBlocking() bool {
	return p.nonBlocking
}

func (p *pool) RunningWorkers() int64 {
	return p.workerCount.Load()
}

func (p *pool) SubmittedTasks() uint64 {
	return p.submittedTaskCount.Load()
}

func (p *pool) WaitingTasks() uint64 {
	return p.tasks.Len()
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

func (p *pool) worker(task any) {
	defer p.workerWaitGroup.Done()

	var readTaskErr, err error
	for {
		_, err = invokeTask[any](task)

		p.updateMetrics(err)

		task, readTaskErr = p.readTask()

		if readTaskErr != nil {
			return
		}
	}
}

func (p *pool) Go(task func()) error {
	return p.submit(task)
}

func (p *pool) Submit(task func()) Task {
	return p.wrapAndSubmit(task)
}

func (p *pool) SubmitErr(task func() error) Task {
	return p.wrapAndSubmit(task)
}

func (p *pool) wrapAndSubmit(task any) Task {
	if p.Stopped() {
		return poolStoppedFuture
	}

	ctx := p.Context()
	future, resolve := future.NewFuture(ctx)

	wrapped := wrapTask[struct{}, func(error)](task, resolve)

	if err := p.submit(wrapped); err != nil {
		resolve(err)
		return future
	}

	return future
}

func (p *pool) submit(task any) error {
	if p.nonBlocking {
		return p.trySubmit(task)
	}

	return p.blockingSubmit(task)
}

func (p *pool) blockingSubmit(task any) error {
	for {
		if err := p.trySubmit(task); err != ErrQueueFull {
			return err
		}

		// No space left in the queue, wait until a slot is released
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-p.submitWaiters:
			select {
			case <-p.ctx.Done():
				return p.ctx.Err()
			default:
			}
		}
	}
}

func (p *pool) trySubmit(task any) error {
	// Check if the pool has been stopped
	if p.Stopped() {
		return ErrPoolStopped
	}

	done := p.ctx.Done()

	var poppedTask any
	var tasksLen int

	p.mutex.Lock()

	// Context was cancelled while waiting for the lock
	select {
	case <-done:
		p.mutex.Unlock()
		return p.ctx.Err()
	default:
	}

	tasksLen = int(p.tasks.Len())

	if p.queueSize > 0 && tasksLen >= p.queueSize {
		p.mutex.Unlock()
		return ErrQueueFull
	}

	if int(p.workerCount.Load()) < p.maxConcurrency {
		p.workerCount.Add(1)

		if tasksLen == 0 {
			// No idle workers and queue is empty, we can pop the task immediately
			poppedTask = task
		} else {
			// Push the task at the back of the queue
			p.tasks.Write(task)

			// Pop the front task
			poppedTask, _ = p.tasks.Read()
		}
	} else {
		// Push the task at the back of the queue
		p.tasks.Write(task)
	}

	p.mutex.Unlock()

	if poppedTask != nil {
		if p.parent == nil {
			p.workerWaitGroup.Add(1)
			// Launch a new worker
			go p.worker(poppedTask)
		} else {
			// Submit task to the parent pool
			p.subpoolSubmit(poppedTask)
		}
	}

	p.submittedTaskCount.Add(1)

	return nil
}

func (p *pool) subpoolSubmit(task any) error {
	p.workerWaitGroup.Add(1)
	return p.parent.submit(func() (output any, err error) {
		defer p.workerWaitGroup.Done()

		output, err = invokeTask[any](task)

		p.updateMetrics(err)

		// Attempt to submit the next task to the parent pool
		if task, err := p.readTask(); err == nil {
			p.subpoolSubmit(task)
		}

		return
	})
}

func (p *pool) readTask() (task any, err error) {
	p.mutex.Lock()

	// Check if the pool context has been cancelled
	select {
	case <-p.ctx.Done():
		err = p.ctx.Err()
		p.mutex.Unlock()
		return
	default:
	}

	if p.tasks.Len() == 0 {
		// No more tasks in the queue, worker will exit
		p.workerCount.Add(-1)
		p.mutex.Unlock()

		err = ErrQueueEmpty
		return
	}

	task, _ = p.tasks.Read()

	p.mutex.Unlock()

	// Notify push waiters that there is space in the queue to push more elements
	p.notifyPushWaiter()

	return
}

func (p *pool) notifyPushWaiter() {
	// Wake up one of the waiters (if any)
	select {
	case p.submitWaiters <- struct{}{}:
	default:
		return
	}
}

func (p *pool) updateMetrics(err error) {
	if err != nil {
		p.failedTaskCount.Add(1)
	} else {
		p.successfulTaskCount.Add(1)
	}
}

func (p *pool) Stop() Task {
	return Submit(func() {
		// Stop accepting new tasks
		p.closed.Store(true)

		// Wait for all workers to finish executing all tasks (including the ones in the queue)
		p.workerWaitGroup.Wait()

		// Cancel the context with a pool stopped error to signal that the pool has been stopped
		p.cancel(ErrPoolStopped)
	})
}

func (p *pool) StopAndWait() {
	p.Stop().Wait()
}

func (p *pool) NewSubpool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, p, options...)
}

func (p *pool) NewGroup() TaskGroup {
	return newTaskGroup(p, p.ctx)
}

func (p *pool) NewGroupContext(ctx context.Context) TaskGroup {
	return newTaskGroup(p, ctx)
}

func newPool(maxConcurrency int, parent *pool, options ...Option) *pool {

	if parent != nil {
		if maxConcurrency > parent.MaxConcurrency() {
			panic(fmt.Errorf("maxConcurrency cannot be greater than the parent pool's maxConcurrency (%d)", parent.MaxConcurrency()))
		}

		if maxConcurrency == 0 {
			maxConcurrency = parent.MaxConcurrency()
		}
	}

	if maxConcurrency == 0 {
		maxConcurrency = math.MaxInt
	}

	if maxConcurrency <= 0 {
		panic(errors.New("maxConcurrency must be greater than 0"))
	}

	pool := &pool{
		ctx:            context.Background(),
		nonBlocking:    DefaultNonBlocking,
		maxConcurrency: maxConcurrency,
		queueSize:      DefaultQueueSize,
		submitWaiters:  make(chan struct{}),
	}

	if parent != nil {
		pool.parent = parent
		pool.ctx = parent.Context()
		pool.queueSize = parent.queueSize
		pool.nonBlocking = parent.nonBlocking
	}

	for _, option := range options {
		option(pool)
	}

	pool.ctx, pool.cancel = context.WithCancelCause(pool.ctx)

	pool.tasks = linkedbuffer.NewLinkedBuffer[any](LinkedBufferInitialSize, LinkedBufferMaxCapacity)

	return pool
}

func NewPool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, nil, options...)
}
