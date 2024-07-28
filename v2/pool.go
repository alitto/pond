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
	Context() context.Context
	Submit(task any) Future[O]
	Go(task any)
	Stop() Future[struct{}]
	Subpool(maxConcurrency int) Pool[O]
	Group() TaskGroup[O]
}

type pool[O any] struct {
	ctx             context.Context
	maxConcurrency  int
	tasks           chan any
	tasksLen        int
	workerCount     atomic.Int64
	workerWaitGroup sync.WaitGroup
	dispatcher      *dispatcher[any]
	// Subpool
	parent    *pool[O]
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

func (p *pool[O]) Context() context.Context {
	return p.ctx
}

func (p *pool[O]) Go(task any) {
	validateTask[O](task)

	p.dispatcher.Write(task)
}

func (p *pool[O]) Submit(task any) Future[O] {
	validateTask[O](task)

	future, resolve := newFuture[O](p.Context())

	futureTask := futureTask[O]{
		task:    task,
		ctx:     future.Context(),
		resolve: resolve,
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

func (p *pool[O]) Subpool(maxConcurrency int) Pool[O] {
	return newPool(p.Context(), maxConcurrency, p)
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

func newPool[O any](ctx context.Context, maxConcurrency int, parent *pool[O]) Pool[O] {
	if ctx == nil {
		panic("context cannot be nil")
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	pool := &pool[O]{
		ctx:            ctx,
		maxConcurrency: maxConcurrency,
		tasks:          make(chan any, tasksLen),
		tasksLen:       tasksLen,
		parent:         parent,
	}

	if parent != nil {
		// Subpool
		pool.sem = make(chan struct{}, maxConcurrency)
		pool.dispatcher = newDispatcher(ctx, pool.subpoolDispatch, tasksLen)
	} else {
		pool.dispatcher = newDispatcher(ctx, pool.dispatch, tasksLen)
	}

	return pool
}

func NewPool[O any](ctx context.Context, maxConcurrency int) Pool[O] {
	return newPool[O](ctx, maxConcurrency, nil)
}
