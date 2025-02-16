package pond

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/dispatcher"
	"github.com/alitto/pond/v2/internal/semaphore"
)

// subpool is a pool that is a subpool of another pool
type subpool struct {
	*pool
	parent    *pool
	waitGroup sync.WaitGroup
	sem       *semaphore.Weighted
}

func newSubpool(maxConcurrency int, ctx context.Context, parent *pool, options ...Option) Pool {

	if maxConcurrency == 0 {
		maxConcurrency = parent.MaxConcurrency()
	}

	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater or equal to 0"))
	}

	if maxConcurrency > parent.MaxConcurrency() {
		panic(fmt.Errorf("maxConcurrency cannot be greater than the parent pool's maxConcurrency (%d)", parent.MaxConcurrency()))
	}

	tasksLen := maxConcurrency
	if tasksLen > MAX_TASKS_CHAN_LENGTH {
		tasksLen = MAX_TASKS_CHAN_LENGTH
	}

	pool := &pool{
		ctx:            ctx,
		maxConcurrency: maxConcurrency,
	}

	for _, option := range options {
		option(pool)
	}

	ctx = pool.Context()

	if pool.queueSize > 0 {
		pool.queueSem = semaphore.NewWeighted(pool.queueSize)
	}

	subpool := &subpool{
		pool:   pool,
		parent: parent,
		sem:    semaphore.NewWeighted(maxConcurrency),
	}

	subpool.pool.dispatcher = dispatcher.NewDispatcher(ctx, subpool.dispatch, tasksLen)

	return subpool
}

func (p *subpool) dispatch(incomingTasks []any) {

	ctx := p.Context()

	// Submit tasks
	for _, task := range incomingTasks {

		// Acquire semaphore to limit concurrency
		if p.nonBlocking {
			if ok := p.sem.TryAcquire(1); !ok {
				return
			}
		} else {
			if err := p.sem.Acquire(ctx, 1); err != nil {
				return
			}
		}

		subpoolTask := subpoolTask[any]{
			task:          task,
			queueSem:      p.queueSem,
			sem:           p.sem,
			waitGroup:     &p.waitGroup,
			updateMetrics: p.updateMetrics,
		}

		p.waitGroup.Add(1)

		if err := p.parent.Go(subpoolTask.Run); err != nil {
			// We failed to submit the task, release semaphore
			subpoolTask.Close()
		}
	}
}

func (p *subpool) Stop() Task {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()
	})
}

func (p *subpool) StopAndWait() {
	p.Stop().Wait()
}

func (p *subpool) RunningWorkers() int64 {
	return int64(p.sem.Acquired())
}
