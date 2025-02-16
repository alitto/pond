package semaphore

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

type waiter struct {
	weight int
	ready  chan struct{}
}

type Weighted struct {
	mutex    sync.Mutex
	waiters  list.List
	size     int
	acquired int
}

func NewWeighted(size int) *Weighted {
	return &Weighted{
		size: size,
	}
}

func (s *Weighted) Acquire(ctx context.Context, weight int) error {
	if weight <= 0 {
		return fmt.Errorf("semaphore: weight %d cannot be negative or zero", weight)
	}
	if weight > s.Size() {
		return fmt.Errorf("semaphore: weight %d is greater than semaphore size %d", weight, s.Size())
	}

	done := ctx.Done()

	// Prioritize context cancellation
	select {
	case <-done:
		return ctx.Err()
	default:
	}

	s.mutex.Lock()

	// Try to acquire the semaphore immediately if there are no waiters
	if s.waiters.Len() == 0 && s.size-s.acquired >= weight {
		s.acquired += weight
		s.mutex.Unlock()
		return nil
	}

	// Wait for the semaphore to be released
	waiter := waiter{
		weight: weight,
		ready:  make(chan struct{}),
	}
	elem := s.waiters.PushBack(waiter)
	s.mutex.Unlock()

	select {
	case <-done:
		select {
		case <-waiter.ready:
			// Acquired the semaphore after we were canceled.
			// Pretend we didn't and put the tokens back.
			s.Release(weight)
		default:
			// We were canceled before acquiring the semaphore.
			s.mutex.Lock()
			isFront := s.waiters.Front() == elem
			s.waiters.Remove(elem)
			// If we're at the front and there're extra tokens left, notify other waiters.
			if isFront && s.size > s.acquired {
				s.notifyWaiters()
			}
			s.mutex.Unlock()
		}
		return ctx.Err()
	case <-waiter.ready:
		// Acquired the semaphore. Check that ctx isn't already done.
		// We check the done channel instead of calling ctx.Err because we
		// already have the channel, and ctx.Err is O(n) with the nesting
		// depth of ctx.
		select {
		case <-done:
			s.Release(weight)
			return ctx.Err()
		default:
		}
		return nil
	}
}

func (s *Weighted) TryAcquire(weight int) bool {
	if weight <= 0 {
		return false
	}
	if weight > s.Size() {
		return false
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Try to acquire the semaphore immediately if there are no waiters
	if s.waiters.Len() == 0 && s.size-s.acquired >= weight {
		s.acquired += weight
		return true
	}

	return false
}

func (s *Weighted) Release(weight int) error {
	if weight <= 0 {
		return fmt.Errorf("semaphore: weight %d cannot be negative or zero", weight)
	}
	if weight > s.Size() {
		return fmt.Errorf("semaphore: weight %d is greater than semaphore size %d", weight, s.Size())
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if weight > s.acquired {
		return fmt.Errorf("semaphore: trying to release more than acquired: %d > %d", weight, s.acquired)
	}

	// Release the semaphore
	s.acquired -= weight

	// Notify waiters
	s.notifyWaiters()

	return nil
}

func (s *Weighted) notifyWaiters() {
	var available int
	for {
		available = s.size - s.acquired
		if available <= 0 {
			break // No more weight available
		}

		next := s.waiters.Front()
		if next == nil {
			break // No more waiters blocked.
		}

		waiter := next.Value.(waiter)
		if waiter.weight > available {
			// Not enough weight to release the waiter
			break
		}

		// Release the waiter
		s.acquired += waiter.weight
		s.waiters.Remove(next)
		close(waiter.ready)
	}
}

func (s *Weighted) Size() int {
	return s.size
}

func (s *Weighted) Acquired() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.acquired
}

func (s *Weighted) Available() int {
	return s.Size() - s.Acquired()
}
