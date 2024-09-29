package pond

import (
	"context"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestPoolOptionsWithContext(t *testing.T) {
	poolOptions := &PoolOptions{}

	assert.Equal(t, nil, poolOptions.Context)

	ctx := context.Background()

	WithContext(ctx)(poolOptions)

	assert.Equal(t, ctx, poolOptions.Context)
}
