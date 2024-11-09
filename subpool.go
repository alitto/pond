package pond

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/alitto/pond/v2/internal/dispatcher"
)

// subpool is a pool that is a subpool of another pool
type subpool struct {
	*pool
	parent    *pool
	waitGroup sync.WaitGroup
	sem       chan struct{}
}

func newSubpool(maxConcurrency int, ctx context.Context, parent *pool) Pool {

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

	subpool := &subpool{
		pool: &pool{
			ctx:            ctx,
			maxConcurrency: maxConcurrency,
		},
		parent: parent,
		sem:    make(chan struct{}, maxConcurrency),
	}

	subpool.pool.dispatcher = dispatcher.NewDispatcher(ctx, subpool.dispatch, tasksLen)

	return subpool
}

func (p *subpool) dispatch(incomingTasks []any) {

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

func (p *subpool) Stop() Task {
	return Submit(func() {
		p.dispatcher.CloseAndWait()

		p.waitGroup.Wait()

		close(p.sem)
	})
}

func (p *subpool) StopAndWait() {
	p.Stop().Wait()
}
