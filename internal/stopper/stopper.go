package stopper

import (
	"sync"
)

func New() *Stopper {
	return &Stopper{
		mu: sync.Mutex{},

		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// Stopper is a WaitGroup-like thread-safe synchronization primitive
// which avoids data races when calling Add during active Wait
type Stopper struct {
	mu  sync.Mutex
	cnt int32

	stoppingOnce sync.Once
	stoppedOnce  sync.Once

	stopping chan struct{}
	stopped  chan struct{}
}

// Add increments internal counter of running jobs
func (s *Stopper) Add(delta int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.stopping:
		return

	default:
	}

	s.cnt += delta
}

// Done decrements internal counter of running jobs
func (s *Stopper) Done() {
	s.mu.Lock()

	if s.cnt <= 0 {
		s.mu.Unlock()

		return
	}

	s.cnt--

	s.mu.Unlock()

	s.checkIfStopped()
}

func (s *Stopper) checkIfStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cnt <= 0 {
		select {
		case <-s.stopping:
			s.stoppedOnce.Do(func() { close(s.stopped) })

		default:
		}
	}
}

// Stop asks all added jobs to stop
func (s *Stopper) Stop() {
	s.stoppingOnce.Do(func() {
		s.mu.Lock()
		close(s.stopping)
		s.mu.Unlock()

		s.checkIfStopped()
	})
}

// Stopping indicates that jobs are stopping or stopped
func (s *Stopper) Stopping() bool {
	select {
	case <-s.stopping:
		return true
	default:
		return false
	}
}

// Stopped indicates that all jobs are stopped
func (s *Stopper) Stopped() bool {
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

// Wait for all jobs to stop
func (s *Stopper) Wait() {
	<-s.stopped
}
