package pond

import (
	"context"
	"errors"
)

var (
	// ErrNilContext is returned when a nil context is provided
	ErrNilContext = errors.New("Context cannot be nil")
)

type PoolOptions struct {
	Context context.Context
}

type PoolOption func(opts *PoolOptions)

func WithContext(ctx context.Context) PoolOption {
	return func(opts *PoolOptions) {
		opts.Context = ctx
	}
}

// Validate validates pool options
func (opts *PoolOptions) Validate() error {
	if opts.Context == nil {
		return ErrNilContext
	}

	return nil
}
