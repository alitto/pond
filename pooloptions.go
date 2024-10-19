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
