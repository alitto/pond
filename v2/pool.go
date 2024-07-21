package pond

import (
	"context"
	"sync"
	"sync/atomic"
)

var DEFAULT_TASKS_CHAN_LENGTH = 2048

type Pool interface {
	Context() context.Context
	Submit(task func())
	Stop() TaskContext[any]
	Subpool(maxConcurrency int) Pool
}

type pool struct {
	ctx             context.Context
	maxConcurrency  int
	tasks           chan func()
	tasksLen        int
	workerCount     atomic.Int64
	workerWaitGroup sync.WaitGroup
	dispatcher      *dispatcher[func()]
}

func (p *pool) Context() context.Context {
	return p.ctx
}

func (p *pool) Submit(task func()) {
	p.dispatcher.Write(task)
}

func (p *pool) Stop() TaskContext[any] {
	return NewTask(func() {
		p.dispatcher.CloseAndWait()
		close(p.tasks)
		p.workerWaitGroup.Wait()
	}).WithContext(p.Context()).Run()
}

func (p *pool) Subpool(maxConcurrency int) Pool {
	return newSubpool(p, maxConcurrency)
}

func (p *pool) dispatch(incomingTasks []func()) {

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
			task()
		default:
			// No tasks left, exit
			return
		}
	}
}

func NewPool(ctx context.Context, maxConcurrency int) Pool {
	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	pool := &pool{
		ctx:            ctx,
		maxConcurrency: maxConcurrency,
		tasks:          make(chan func(), tasksLen),
		tasksLen:       tasksLen,
	}

	pool.dispatcher = newDispatcher(ctx, pool.dispatch, int(tasksLen))

	return pool
}
