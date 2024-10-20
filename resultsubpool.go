package pond

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/dispatcher"
)

type resultSubpool[R any] struct {
	*resultPool[R]
	parent    *pool
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

func newResultSubpool[R any](maxConcurrency int, ctx context.Context, parent *pool) ResultPool[R] {

	if maxConcurrency == 0 {
		maxConcurrency = parent.MaxConcurrency()
	}

	if maxConcurrency < 0 {
		panic(errors.New("maxConcurrency must be greater or equal to 0"))
	}

	if maxConcurrency > parent.MaxConcurrency() {
		panic(fmt.Errorf("maxConcurrency cannot be greater than the parent pool's maxConcurrency (%d)", parent.MaxConcurrency()))
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	subpool := &resultSubpool[R]{
		resultPool: &resultPool[R]{
			pool: &pool{
				ctx:            ctx,
				maxConcurrency: maxConcurrency,
			},
		},
		parent: parent,
		sem:    make(chan struct{}, maxConcurrency),
	}

	subpool.pool.dispatcher = dispatcher.NewDispatcher(ctx, subpool.dispatch, tasksLen)

	return subpool
}

func (p *resultSubpool[R]) dispatch(incomingTasks []any) {

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

		subpoolTask := subpoolTask[any]{
			task:          task,
			sem:           p.sem,
			waitGroup:     &p.waitGroup,
			updateMetrics: p.updateMetrics,
		}

		p.parent.Go(subpoolTask.Run)
	}
}

func (p *resultSubpool[R]) Stop() Task {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()

		close(p.sem)
	})
}

func (p *resultSubpool[R]) StopAndWait() {
	p.Stop().Wait()
}
