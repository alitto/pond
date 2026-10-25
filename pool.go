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
	// Constant for an unbounded queue
	Unbounded               = math.MaxInt
	DefaultQueueSize        = Unbounded
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

// BasePool defines methods common to all pool types.
type BasePool interface {
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
	// Tasks accepted by the pool but canceled before execution are excluded.
	CompletedTasks() uint64

	// Returns the number of tasks that have been dropped because the queue was full.
	DroppedTasks() uint64

	// Returns the number of tasks accepted by the pool that were canceled
	// before executing user code due to pool context cancellation.
	CanceledTasks() uint64

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
	BasePool

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
	panicRecovery       bool
	maxConcurrency      int
	closed              atomic.Bool
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	submitWaiters       chan struct{}
	stopWaiters         chan struct{}
	queueSize           int
	tasks               *linkedbuffer.LinkedBuffer[any]
	submittedTaskCount  atomic.Uint64
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
	droppedTaskCount    atomic.Uint64
	canceledTaskCount   atomic.Uint64
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

	// Workers from an earlier limit can still be running after a shrink.
	newWorkers := maxConcurrency - int(p.workerCount.Load())
	if queuedTasks := int(p.tasks.Len()); newWorkers > queuedTasks {
		newWorkers = queuedTasks
	}

	p.maxConcurrency = maxConcurrency

	if newWorkers > 0 {
		p.workerCount.Add(int64(newWorkers))
		p.workerWaitGroup.Add(newWorkers)
	}

	p.mutex.Unlock()

	// Launch the new workers
	for i := 0; i < newWorkers; i++ {
		if err := p.launchWorker(nil); err != nil {
			p.handleParentStopped(nil, err)
		}
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
	p.mutex.Lock()
	defer p.mutex.Unlock()

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

func (p *pool) CanceledTasks() uint64 {
	return p.canceledTaskCount.Load()
}

func (p *pool) worker(task any) {
	var readTaskErr, err error
	exitedNormally := false
	defer func() {
		if !exitedNormally {
			// In case of abnormal exit (e.g. runtime.Goexit() in the task),
			// launch a new worker to execute the next task in the queue.
			p.updateMetrics(fmt.Errorf("worker exited abnormally: %w", err))

			task, err := p.readTask()
			if err != nil {
				return
			}

			if task != nil {
				p.launchWorker(task)
				p.notifySubmitWaiter()
			}
		}
	}()
	for {
		if task != nil {
			_, err = invokeTask[any](task, p.panicRecovery)

			p.updateMetrics(err)
		}

		task, readTaskErr = p.readTask()

		if readTaskErr != nil {
			exitedNormally = true
			return
		}
	}
}

func (p *pool) subpoolWorker(task any) func() (output any, err error) {
	return func() (output any, err error) {
		exitedNormally := false
		defer func() {
			if !exitedNormally {
				p.updateMetrics(fmt.Errorf("worker exited abnormally: %w", err))
				if nextTask, readErr := p.readTask(); readErr == nil {
					if launchErr := p.launchWorker(nextTask); launchErr != nil {
						p.handleParentStopped(nextTask, launchErr)
					}
				}
			}
		}()

		if task != nil {
			output, err = invokeTask[any](task, p.panicRecovery)

			p.updateMetrics(err)
		}

		exitedNormally = true

		// Submit the next child task without losing it if a non-blocking parent is full.
		if nextTask, readErr := p.readTask(); readErr == nil {
			if launchErr := p.launchWorker(nextTask); launchErr != nil {
				p.handleParentStopped(nextTask, launchErr)
			}
		}

		return
	}
}

func (p *pool) Go(task func()) error {
	// Wrap the task so it is skipped if the pool context is canceled while it is queued
	return p.submit(wrapTask[struct{}, func(error)](task, func(error) {}, p.ctx, p.panicRecovery), p.nonBlocking)
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

	wrappedTask := wrapTask[struct{}, func(error)](task, resolve, ctx, p.panicRecovery)

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

	queueEnabled := p.queueSize > 0
	tasksLen := int(p.tasks.Len())

	// When queue is enabled, check if it is full
	if queueEnabled && tasksLen >= p.queueSize {
		p.mutex.Unlock()
		return ErrQueueFull
	}

	if int(p.workerCount.Load()) >= p.maxConcurrency {
		// When queue is disabled, return an error immediately if max concurrency is reached
		if !queueEnabled {
			p.mutex.Unlock()
			return ErrQueueFull
		}

		// If queue is enabled, push the task at the back of the queue
		p.tasks.Write(task)
		p.mutex.Unlock()
		return nil
	}

	p.workerCount.Add(1)
	p.workerWaitGroup.Add(1)

	if queueEnabled && tasksLen > 0 {
		// Push the task at the back of the queue
		p.tasks.Write(task)

		// Pop the front task
		task, _ = p.tasks.Read()
	}

	p.mutex.Unlock()

	if err := p.launchWorker(task); err != nil {
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.notifySubmitWaiter()
		return err
	}

	// Notify a submit waiter there is room in the queue for a new task
	p.notifySubmitWaiter()

	return nil
}

func (p *pool) launchWorker(task any) error {
	if p.parent == nil {
		// Launch a new worker to execute the task
		go p.worker(task)
		return nil
	}

	// Submit the child worker to the parent pool.
	parentTask := p.subpoolWorker(task)
	if !p.nonBlocking {
		return p.parent.submit(parentTask, false)
	}

	// A full parent queue must not orphan a task already accepted by the subpool.
	p.parent.submittedTaskCount.Add(1)
	if err := p.parent.trySubmit(parentTask); err != ErrQueueFull {
		if err != nil {
			p.parent.droppedTaskCount.Add(1)
		}
		return err
	}

	if err := p.scheduleSubpoolWorker(task, parentTask); err != nil {
		p.parent.droppedTaskCount.Add(1)
		return err
	}
	return nil
}

func (p *pool) scheduleSubpoolWorker(task any, parentTask any) error {
	parent := p.parent
	parent.mutex.Lock()
	if parent.Stopped() {
		parent.mutex.Unlock()
		return ErrPoolStopped
	}
	// Hold a parent worker wait-group reservation until the worker wrapper is
	// either queued/launched or the parent has stopped accepting work.
	parent.workerWaitGroup.Add(1)
	parent.mutex.Unlock()

	go func() {
		err := p.blockingSubmitSubpoolWorker(parentTask)
		if err != nil {
			parent.droppedTaskCount.Add(1)
			p.handleParentStopped(task, err)
		}
		parent.workerWaitGroup.Done()
	}()
	return nil
}

func (p *pool) blockingSubmitSubpoolWorker(parentTask any) error {
	parent := p.parent
	for {
		if p.Stopped() || parent.Stopped() {
			return ErrPoolStopped
		}
		if err := parent.trySubmit(parentTask); err != ErrQueueFull {
			return err
		}

		select {
		case <-parent.submitWaiters:
		case <-p.stopWaiters:
			return ErrPoolStopped
		case <-parent.stopWaiters:
			return ErrPoolStopped
		case <-p.ctx.Done():
			return ErrPoolStopped
		case <-parent.ctx.Done():
			return ErrPoolStopped
		}
	}
}

func (p *pool) handleParentStopped(task any, err error) {
	// Cancel the subpool context and drain accepted tasks; their futures resolve without running user code.
	canceledErr := errors.Join(ErrContextCanceled, err)
	p.cancel(err)
	if task != nil {
		p.updateMetrics(canceledErr)
		p.parent.updateMetrics(canceledErr)
	}

	p.mutex.Lock()
	for p.tasks.Len() > 0 {
		p.tasks.Read()
		p.updateMetrics(canceledErr)
		p.parent.updateMetrics(canceledErr)
	}
	p.workerCount.Add(-1)
	p.workerWaitGroup.Done()
	p.mutex.Unlock()

	p.notifySubmitWaiter()
}

func (p *pool) readTask() (task any, err error) {
	p.mutex.Lock()

	if p.tasks.Len() == 0 {
		// No more tasks in the queue, worker will exit
		p.workerCount.Add(-1)
		p.workerWaitGroup.Done()
		p.mutex.Unlock()

		// Notify a submit waiter there is room in the queue for a new task
		p.notifySubmitWaiter()

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
		if errors.Is(err, ErrContextCanceled) {
			p.canceledTaskCount.Add(1)
		} else {
			p.failedTaskCount.Add(1)
		}
	} else {
		p.successfulTaskCount.Add(1)
	}
}

func (p *pool) Stop() Task {
	return Submit(func() {
		// Stop accepting new tasks while holding the lock to avoid race conditions.
		p.mutex.Lock()
		if !p.closed.Load() {
			p.closed.Store(true)
			close(p.stopWaiters)
		}
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
		panicRecovery:  true,
		maxConcurrency: maxConcurrency,
		queueSize:      DefaultQueueSize,
		// Buffer size of 1 to prevent deadlock when read on the submitWaiters channel happens
		// after the write on the same channel in the notifySubmitWaiter method.
		// See https://github.com/alitto/pond/issues/108
		submitWaiters: make(chan struct{}, 1),
		stopWaiters:   make(chan struct{}),
	}

	if parent != nil {
		pool.parent = parent
		pool.ctx = parent.Context()
		pool.queueSize = parent.queueSize
		pool.nonBlocking = parent.nonBlocking
		pool.panicRecovery = parent.panicRecovery
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
