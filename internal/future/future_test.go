package future

import (
	"context"
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestFutureGet(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewFuture[int](ctx)

	resolve(5, nil)

	out, err := future.Get()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, out)
	assert.Equal(t, context.Canceled, future.Context().Err())

	err = future.Wait()

	assert.Equal(t, nil, err)
}

func TestFutureGetWithError(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewFuture[int](ctx)

	err := errors.New("sample error")

	resolve(0, err)

	out, err := future.Get()

	assert.Equal(t, err, err)
	assert.Equal(t, 0, out)

	err = future.Wait()

	assert.Equal(t, err, err)
}

func TestFutureGetWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := NewFuture[int](ctx)

	cancel()

	resolve(0, nil)

	out, err := future.Get()

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 0, out)

	err = future.Wait()

	assert.Equal(t, context.Canceled, err)
}
