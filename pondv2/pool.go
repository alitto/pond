package pondv2

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond/unboundedchanv2"
)

var DEFAULT_TASKS_CHAN_LENGTH = 2048

type Task = func()

type WorkerPool interface {
	Submit(task Task)
	StopAndWait()
}

type workerPool struct {
	ctx             context.Context
	maxWorkers      int
	tasks           chan Task
	tasksLen        int
	workerCount     atomic.Int64
	workerWaitGroup sync.WaitGroup
	inputChan       *unboundedchanv2.UnboundedChan[Task]
}

func (p *workerPool) Submit(task Task) {
	p.inputChan.Write(task)
}

func (p *workerPool) StopAndWait() {
	p.inputChan.CloseAndWait()
	close(p.tasks)
	p.workerWaitGroup.Wait()
}

func (p *workerPool) dispatch(incomingTasks []Task) {

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
		if workerCount < p.maxWorkers {
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

func (p *workerPool) startWorker() {
	p.workerWaitGroup.Add(1)
	p.workerCount.Add(1)
	go p.worker()
}

func (p *workerPool) worker() {
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

func NewPool(ctx context.Context, maxWorkers int) WorkerPool {
	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxWorkers < tasksLen {
		tasksLen = maxWorkers
	}

	pool := &workerPool{
		ctx:        ctx,
		maxWorkers: maxWorkers,
		tasks:      make(chan Task, tasksLen),
		tasksLen:   tasksLen,
	}

	pool.inputChan = unboundedchanv2.NewUnboundedChan(ctx, pool.dispatch, int(tasksLen))

	return pool
}
