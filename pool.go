package pond

import (
	"context"
	"sync"
	"sync/atomic"
)

const DEFAULT_TASKS_CHAN_LENGTH = 2048

/**
 * Interface implemented by all worker pools in this library.
 * @param O The type of the output of the tasks submitted to the pool
 */
type Pool[O any] interface {
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

	// Returns the context associated with this pool.
	Context() context.Context

	// Submits a task to the pool and returns a future that can be used to wait for the task to complete.
	Submit(task any) Future[O]

	// Submits a task to the pool without waiting for it to complete.
	Go(task any)

	// Stops the pool and returns a future that can be used to wait for all tasks pending to complete.
	Stop() Future[struct{}]

	// Stops the pool and waits for all tasks to complete.
	StopAndWait()

	// Creates a new subpool with the specified maximum concurrency.
	Subpool(maxConcurrency int, options ...PoolOption) Pool[O]

	// Creates a new task group.
	Group() TaskGroup[O]
}

// pool is an implementation of the Pool interface.
type pool[O any] struct {
	ctx                 context.Context
	maxConcurrency      int
	tasks               chan any
	tasksLen            int
	workerCount         atomic.Int64
	workerWaitGroup     sync.WaitGroup
	dispatcher          *dispatcher[any]
	successfulTaskCount atomic.Uint64
	failedTaskCount     atomic.Uint64
	// Subpool properties
	parent    *pool[O]
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

func (p *pool[O]) Context() context.Context {
	return p.ctx
}

func (p *pool[O]) RunningWorkers() int64 {
	return p.workerCount.Load()
}

func (p *pool[O]) SubmittedTasks() uint64 {
	return p.dispatcher.WriteCount()
}

func (p *pool[O]) WaitingTasks() uint64 {
	return p.dispatcher.Len()
}

func (p *pool[O]) FailedTasks() uint64 {
	return p.failedTaskCount.Load()
}

func (p *pool[O]) SuccessfulTasks() uint64 {
	return p.successfulTaskCount.Load()
}

func (p *pool[O]) CompletedTasks() uint64 {
	return p.successfulTaskCount.Load() + p.failedTaskCount.Load()
}

func (p *pool[O]) Go(task any) {
	validateTask[O](task)

	p.dispatcher.Write(task)
}

func (p *pool[O]) Submit(task any) Future[O] {
	validateTask[O](task)

	future, resolve := newFuture[O](p.Context())

	futureTask := futureTask[O]{
		task: task,
		ctx:  future.Context(),
		resolve: func(output O, err error) {
			resolve(output, err)

			// Update counters
			if err != nil {
				p.failedTaskCount.Add(1)
			} else {
				p.successfulTaskCount.Add(1)
			}
		},
	}

	p.dispatcher.Write(futureTask.Run)

	return future
}

func (p *pool[O]) Stop() Future[struct{}] {
	return Submit[struct{}](func() {
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
func (p *pool[O]) StopAndWait() {
	p.Stop().Wait()
}

func (p *pool[O]) Subpool(maxConcurrency int, options ...PoolOption) Pool[O] {

	opts := &PoolOptions{
		Context: p.ctx,
	}

	for _, option := range options {
		option(opts)
	}

	return newPool(maxConcurrency, opts, p)
}

func (p *pool[O]) Group() TaskGroup[O] {
	return &taskGroup[O]{
		pool: p,
	}
}

func (p *pool[O]) dispatch(incomingTasks []any) {

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

func (p *pool[O]) subpoolDispatch(incomingTasks []any) {

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

		subpoolTask := subpoolTask[O]{
			task:      task,
			sem:       p.sem,
			waitGroup: &p.waitGroup,
		}

		p.parent.Go(subpoolTask.Run)
	}
}

func (p *pool[O]) startWorker() {
	p.workerWaitGroup.Add(1)
	p.workerCount.Add(1)
	go p.worker()
}

func (p *pool[O]) worker() {
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
			invokeTask[O](task, p.Context())

		default:
			// No tasks left, exit
			return
		}
	}
}

func newPool[O any](maxConcurrency int, options *PoolOptions, parent *pool[O]) Pool[O] {

	if maxConcurrency <= 0 {
		panic("maxConcurrency must be greater than 0")
	}

	if err := options.Validate(); err != nil {
		panic("invalid pool options: " + err.Error())
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	pool := &pool[O]{
		ctx:            options.Context,
		maxConcurrency: maxConcurrency,
		tasks:          make(chan any, tasksLen),
		tasksLen:       tasksLen,
		parent:         parent,
	}

	if parent != nil {
		// Subpool
		pool.sem = make(chan struct{}, maxConcurrency)
		pool.dispatcher = newDispatcher(options.Context, pool.subpoolDispatch, tasksLen)
	} else {
		pool.dispatcher = newDispatcher(options.Context, pool.dispatch, tasksLen)
	}

	return pool
}

// NewPool creates a new pool with the specified maximum concurrency and options.
func NewPool(maxConcurrency int, options ...PoolOption) Pool[any] {
	return NewTypedPool[any](maxConcurrency, options...)
}

// NewTypedPool creates a new pool with the specified maximum concurrency and options, with a specific output type.
func NewTypedPool[O any](maxConcurrency int, options ...PoolOption) Pool[O] {

	opts := &PoolOptions{
		Context: context.Background(),
	}

	for _, option := range options {
		option(opts)
	}

	return newPool[O](maxConcurrency, opts, nil)
}
