package future

import (
	"context"
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestValueFutureWait(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewValueFuture[int](ctx)

	resolve(5, nil)

	out, err := future.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 5, out)
}

func TestValueFutureWaitWithError(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewValueFuture[int](ctx)

	err := errors.New("sample error")

	resolve(0, err)

	out, err := future.Wait()

	assert.Equal(t, err, err)
	assert.Equal(t, 0, out)
}

func TestValueFutureWaitWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := NewValueFuture[int](ctx)

	cancel()

	resolve(0, nil)

	out, err := future.Wait()

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 0, out)
}
