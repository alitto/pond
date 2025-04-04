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
	ErrQueueFull             = errors.New("queue is full")
	ErrQueueEmpty            = errors.New("queue is empty")
	ErrPoolStopped           = errors.New("pool stopped")
	ErrMaxConcurrencyReached = errors.New("max concurrency reached")

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

	// Returns the number of tasks that have been dropped because the queue was full.
	DroppedTasks() uint64

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
	// The pool will not accept new tasks after it has been stopped.
	Stop() Task

	// Stops the pool and waits for all tasks to complete.
	StopAndWait()

	// Returns true if the pool has been stopped or its context has been cancelled.
	Stopped() bool

	// Resizes the pool by changing the maximum concurrency (number of workers) of the pool.
	// The new max concurrency must be greater than 0.
	// If the new max concurrency is less than the current number of running workers, the pool will continue to run with the new max concurrency.
	Resize(maxConcurrency int)
}

// Represents a pool of goroutines that can execute tasks concurrently.
type Pool interface {
	basePool

	// Submits a task to the pool without waiting for it to complete.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, this method will return ErrPoolStopped.
	Go(task func()) error

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, the returned future will resolve to ErrPoolStopped.
	Submit(task func()) Task

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	// The task function must return an error.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, the returned future will resolve to ErrPoolStopped.
	SubmitErr(task func() error) Task

	// Attempts to submit a task to the pool and returns a future that can be used to wait for the task to complete
	// and a boolean indicating whether the task was submitted successfully.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, the returned future will resolve to ErrPoolStopped.
	TrySubmit(task func()) (Task, bool)

	// Attempts to submit a task to the pool and returns a future that can be used to wait for the task to complete
	// and a boolean indicating whether the task was submitted successfully.
	// The task function must return an error.
	// The pool will not accept new tasks after it has been stopped.
	// If the pool has been stopped, the returned future will resolve to ErrPoolStopped.
	TrySubmitErr(task func() error) (Task, bool)

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
	droppedTaskCount    atomic.Uint64
}

func (p *pool) Context() context.Context {
	return p.ctx
}

func (p *pool) Stopped() bool {
	return p.closed.Load() || p.ctx.Err() != nil
}

func (p *pool) MaxConcurrency() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.maxConcurrency
}

func (p *pool) Resize(maxConcurrency int) {
	if maxConcurrency == 0 {
		maxConcurrency = math.MaxInt
	}

	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater than or equal to 0"))
	}

	p.mutex.Lock()

	// Calculate the number of new workers to launch to reach the new max concurrency or the number of tasks in the queue, whichever is smaller
	newWorkers := int(math.Min(float64(maxConcurrency-p.maxConcurrency), float64(p.tasks.Len())))

	p.maxConcurrency = maxConcurrency

	if newWorkers > 0 {
		p.workerCount.Add(int64(newWorkers))
		p.workerWaitGroup.Add(newWorkers)
	}

	p.mutex.Unlock()

	// Launch the new workers
	for i := 0; i < newWorkers; i++ {
		p.launchWorker(nil)
	}
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

func (p *pool) DroppedTasks() uint64 {
	return p.droppedTaskCount.Load()
}

func (p *pool) worker(task any) {
	var readTaskErr, err error
	for {
		if task != nil {
			_, err = invokeTask[any](task)

			p.updateMetrics(err)
		}

		task, readTaskErr = p.readTask()

		if readTaskErr != nil {
			return
		}
	}
}

func (p *pool) subpoolWorker(task any) func() (output any, err error) {
	return func() (output any, err error) {
		if task != nil {
			output, err = invokeTask[any](task)

			p.updateMetrics(err)
		}

		// Attempt to submit the next task to the parent pool
		if task, err := p.readTask(); err == nil {
			p.parent.submit(p.subpoolWorker(task), p.nonBlocking)
		}

		return
	}
}

func (p *pool) Go(task func()) error {
	return p.submit(task, p.nonBlocking)
}

func (p *pool) Submit(task func()) Task {
	future, _ := p.wrapAndSubmit(task, p.nonBlocking)
	return future
}

func (p *pool) SubmitErr(task func() error) Task {
	future, _ := p.wrapAndSubmit(task, p.nonBlocking)
	return future
}

func (p *pool) TrySubmit(task func()) (Task, bool) {
	return p.wrapAndSubmit(task, true)
}

func (p *pool) TrySubmitErr(task func() error) (Task, bool) {
	return p.wrapAndSubmit(task, true)
}

func (p *pool) wrapAndSubmit(task any, nonBlocking bool) (Task, bool) {
	if p.Stopped() {
		return poolStoppedFuture, false
	}

	future, wrappedTask, resolve := p.wrapTask(task)

	if err := p.submit(wrappedTask, nonBlocking); err != nil {
		resolve(err)
		return future, false
	}

	return future, true
}

func (p *pool) wrapTask(task any) (Task, func() error, func(error)) {
	ctx := p.Context()
	future, resolve := future.NewFuture(ctx)

	wrappedTask := wrapTask[struct{}, func(error)](task, resolve)

	return future, wrappedTask, resolve
}

func (p *pool) submit(task any, nonBlocking bool) (err error) {

	p.submittedTaskCount.Add(1)

	if nonBlocking {
		err = p.trySubmit(task)
	} else {
		err = p.blockingTrySubmit(task)
	}

	if err != nil {
		p.droppedTaskCount.Add(1)
	}

	return
}

func (p *pool) blockingTrySubmit(task any) error {
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
	p.mutex.Lock()

	// Check if the pool has been stopped while holding the lock
	// to avoid race conditions on the workers wait group if the pool is being stopped.
	if p.Stopped() {
		p.mutex.Unlock()
		return ErrPoolStopped
	}

	tasksLen := int(p.tasks.Len())

	if p.queueSize > 0 && tasksLen >= p.queueSize {
		p.mutex.Unlock()
		return ErrQueueFull
	}

	if int(p.workerCount.Load()) >= p.maxConcurrency {
		// Push the task at the back of the queue
		p.tasks.Write(task)

		p.mutex.Unlock()

		return nil
	}

	p.workerCount.Add(1)
	p.workerWaitGroup.Add(1)

	if tasksLen > 0 {
		// Push the task at the back of the queue
		p.tasks.Write(task)

		// Pop the front task
		task, _ = p.tasks.Read()
	}

	p.mutex.Unlock()

	p.launchWorker(task)

	return nil
}

func (p *pool) launchWorker(task any) {
	if p.parent == nil {
		// Launch a new worker to execute the task
		go p.worker(task)
	} else {
		// Submit task to the parent pool wrapped in a function that will
		// submit the next task to the parent pool when it completes (subpool worker)
		p.parent.submit(p.subpoolWorker(task), p.nonBlocking)
	}
}

func (p *pool) readTask() (task any, err error) {
	p.mutex.Lock()

	// Check if the pool context has been cancelled
	select {
	case <-p.ctx.Done():
		// Context cancelled, worker will exit
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()

		err = p.ctx.Err()
		return
	default:
	}

	if p.tasks.Len() == 0 {
		// No more tasks in the queue, worker will exit
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()

		err = ErrQueueEmpty
		return
	}

	if p.maxConcurrency > 0 && int(p.workerCount.Load()) > p.maxConcurrency {
		// Max concurrency reached, kill the worker
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()

		err = ErrMaxConcurrencyReached
		return
	}

	task, _ = p.tasks.Read()

	p.mutex.Unlock()

	// Notify a submit waiter there is room in the queue for a new task
	p.notifySubmitWaiter()

	return
}

func (p *pool) notifySubmitWaiter() {
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
		// Stop accepting new tasks while holding the lock to avoid race conditions.
		p.mutex.Lock()
		p.closed.Store(true)
		p.mutex.Unlock()

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

	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater than or equal to 0"))
	}

	pool := &pool{
		ctx:            context.Background(),
		nonBlocking:    DefaultNonBlocking,
		maxConcurrency: maxConcurrency,
		queueSize:      DefaultQueueSize,
		// Buffer size of 1 to prevent deadlock when read on the submitWaiters channel happens
		// after the write on the same channel in the notifySubmitWaiter method.
		// See https://github.com/alitto/pond/issues/108
		submitWaiters: make(chan struct{}, 1),
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

// NewPool creates a new pool with the given maximum concurrency and options.
// The new maximum concurrency must be greater than or equal to 0 (0 means no limit).
func NewPool(maxConcurrency int, options ...Option) Pool {
	return newPool(maxConcurrency, nil, options...)
}
