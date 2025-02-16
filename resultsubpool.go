package pond

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/dispatcher"
	"github.com/alitto/pond/v2/internal/semaphore"
)

type resultSubpool[R any] struct {
	*resultPool[R]
	parent    *pool
	waitGroup sync.WaitGroup
	sem       *semaphore.Weighted
}

func newResultSubpool[R any](maxConcurrency int, ctx context.Context, parent *pool, options ...Option) ResultPool[R] {

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

	resultPool := &resultPool[R]{
		pool: &pool{
			ctx:            ctx,
			maxConcurrency: maxConcurrency,
		},
	}

	for _, option := range options {
		option(resultPool.pool)
	}

	ctx = resultPool.Context()

	if resultPool.pool.queueSize > 0 {
		resultPool.pool.queueSem = semaphore.NewWeighted(resultPool.pool.queueSize)
	}

	subpool := &resultSubpool[R]{
		resultPool: resultPool,
		parent:     parent,
		sem:        semaphore.NewWeighted(maxConcurrency),
	}

	subpool.pool.dispatcher = dispatcher.NewDispatcher(ctx, subpool.dispatch, tasksLen)

	return subpool
}

func (p *resultSubpool[R]) dispatch(incomingTasks []any) {

	ctx := p.Context()

	// Submit tasks
	for _, task := range incomingTasks {

		// Acquire semaphore to limit concurrency
		if p.nonBlocking {
			if ok := p.sem.TryAcquire(1); !ok {
				// Context canceled, exit
				return
			}
		} else {
			if err := p.sem.Acquire(ctx, 1); err != nil {
				// Context canceled, exit
				return
			}
		}

		subpoolTask := subpoolTask[any]{
			task:          task,
			sem:           p.sem,
			queueSem:      p.queueSem,
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

func (p *resultSubpool[R]) Stop() Task {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()
	})
}

func (p *resultSubpool[R]) StopAndWait() {
	p.Stop().Wait()
}

func (p *resultSubpool[R]) RunningWorkers() int64 {
	return int64(p.sem.Acquired())
}
