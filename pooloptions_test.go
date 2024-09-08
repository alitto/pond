package pond

import (
	"context"
	"testing"
)

func TestPoolOptionsWithContext(t *testing.T) {
	poolOptions := &PoolOptions{}

	assertEqual(t, nil, poolOptions.Context)

	ctx := context.Background()

	WithContext(ctx)(poolOptions)

	assertEqual(t, ctx, poolOptions.Context)
}
