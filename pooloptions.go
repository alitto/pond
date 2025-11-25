package pond

import (
	"context"
)

type Option func(*pool)

// WithContext sets the context for the pool.
func WithContext(ctx context.Context) Option {
	return func(p *pool) {
		p.ctx = ctx
	}
}

// WithQueueSize sets the max number of elements that can be queued in the pool.
func WithQueueSize(size int) Option {
	return func(p *pool) {
		p.queueSize = size
	}
}

// WithNonBlocking sets the pool to be non-blocking when the queue is full.
// This option is only effective when the queue size is set.
func WithNonBlocking(nonBlocking bool) Option {
	return func(p *pool) {
		p.nonBlocking = nonBlocking
	}
}

// WithoutPanicRecovery disables panic interception inside worker goroutines.
// When this option is enabled, panics inside tasks will propagate just like regular goroutines.
func WithoutPanicRecovery() Option {
	return func(p *pool) {
		p.panicRecovery = false
	}
}
