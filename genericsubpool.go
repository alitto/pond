package pond

import (
	"context"
	"sync"

	"github.com/alitto/pond/v2/internal/dispatcher"
)

type genericSubpool[R any] struct {
	*genericPool[R]
	parent    Pool
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

// Subpool creates a new pool with the specified maximum concurrency.
func newGenericSubpool[R any](maxConcurrency int, ctx context.Context, parent Pool) GenericPool[R] {

	if maxConcurrency == 0 {
		maxConcurrency = parent.MaxConcurrency()
	}

	if maxConcurrency < 0 {
		panic("maxConcurrency must be greater than 0")
	}

	if maxConcurrency > parent.MaxConcurrency() {
		panic("maxConcurrency must be less than or equal to the parent pool's maxConcurrency")
	}

	tasksLen := DEFAULT_TASKS_CHAN_LENGTH
	if maxConcurrency < tasksLen {
		tasksLen = maxConcurrency
	}

	subpool := &genericSubpool[R]{
		genericPool: &genericPool[R]{
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

func (p *genericSubpool[R]) dispatch(incomingTasks []any) {

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

func (p *genericSubpool[R]) Stop() TaskFuture {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()

		close(p.sem)
	})
}
