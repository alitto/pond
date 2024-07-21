package pond

import (
	"context"
	"sync"
)

type subpool struct {
	pool           Pool
	dispatcher     *dispatcher[func()]
	maxConcurrency int
	sem            chan struct{}
	waitGroup      sync.WaitGroup
}

func (p *subpool) Context() context.Context {
	return p.pool.Context()
}

func (p *subpool) Subpool(maxConcurrency int) Pool {
	return newSubpool(p, maxConcurrency)
}

func (p *subpool) Submit(task func()) {
	p.dispatcher.Write(task)
}

func (p *subpool) Stop() TaskContext[any] {
	return NewTask(func() {
		p.dispatcher.CloseAndWait()
		p.waitGroup.Wait()
		close(p.sem)
	}).WithContext(p.Context()).Run()
}

func (p *subpool) dispatch(incomingTasks []func()) {

	p.waitGroup.Add(len(incomingTasks))

	// Submit tasks
	for _, task := range incomingTasks {

		select {
		case <-p.pool.Context().Done():
			// Context canceled, exit
			return
		case p.sem <- struct{}{}:
			// Acquired the semaphore, submit another task
		}

		p.pool.Submit(func() {
			defer func() {
				// Release semaphore
				<-p.sem
				// Decrement wait group
				p.waitGroup.Done()
			}()
			task()
		})
	}
}

func newSubpool(pool Pool, maxConcurrency int) Pool {
	sp := &subpool{
		pool:           pool,
		maxConcurrency: maxConcurrency,
		sem:            make(chan struct{}, maxConcurrency),
	}

	sp.dispatcher = newDispatcher(pool.Context(), sp.dispatch, maxConcurrency)

	return sp
}
