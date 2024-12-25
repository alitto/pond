package semaphore

import (
	"context"
	"fmt"
	"sync"
)

type Weighted struct {
	ctx     context.Context
	cond    *sync.Cond
	size    int
	n       int
	waiting int
}

func NewWeighted(ctx context.Context, size int) *Weighted {
	sem := &Weighted{
		ctx:  ctx,
		cond: sync.NewCond(&sync.Mutex{}),
		size: size,
		n:    size,
	}

	// Notify all waiters when the context is done
	context.AfterFunc(ctx, func() {
		sem.cond.Broadcast()
	})

	return sem
}

func (w *Weighted) Acquire(weight int) error {
	if weight <= 0 {
		return fmt.Errorf("semaphore: weight %d cannot be negative or zero", weight)
	}
	if weight > w.size {
		return fmt.Errorf("semaphore: weight %d is greater than semaphore size %d", weight, w.size)
	}

	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	done := w.ctx.Done()

	select {
	case <-done:
		return w.ctx.Err()
	default:
	}

	for weight > w.n {
		// Check if the context is done
		select {
		case <-done:
			return w.ctx.Err()
		default:
		}

		w.waiting++
		w.cond.Wait()
		w.waiting--
	}

	w.n -= weight

	return nil
}

func (w *Weighted) TryAcquire(weight int) bool {
	if weight <= 0 {
		return false
	}
	if weight > w.size {
		return false
	}

	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	// Check if the context is done
	select {
	case <-w.ctx.Done():
		return false
	default:
	}

	if weight > w.n {
		// Not enough room in the semaphore
		return false
	}

	w.n -= weight

	return true
}

func (w *Weighted) Release(weight int) error {
	if weight <= 0 {
		return fmt.Errorf("semaphore: weight %d cannot be negative or zero", weight)
	}
	if weight > w.size {
		return fmt.Errorf("semaphore: weight %d is greater than semaphore size %d", weight, w.size)
	}

	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	if weight > w.size-w.n {
		return fmt.Errorf("semaphore: trying to release more than acquired: %d > %d", weight, w.size-w.n)
	}

	w.n += weight
	w.cond.Broadcast()

	return nil
}

func (w *Weighted) Size() int {
	return w.size
}

func (w *Weighted) Acquired() int {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	return w.size - w.n
}

func (w *Weighted) Available() int {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	return w.n
}

func (w *Weighted) Waiting() int {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()

	return w.waiting
}
