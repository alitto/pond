package future

import (
	"context"
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestFutureWait(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewFuture(ctx)

	resolve(nil)

	err := future.Wait()

	assert.Equal(t, nil, err)
}

func TestFutureWaitWithError(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewFuture(ctx)

	sampleErr := errors.New("sample error")

	resolve(sampleErr)

	err := future.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, "sample error", err.Error())
}

func TestFutureWaitWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := NewFuture(ctx)

	cancel()

	resolve(errors.New("sample error"))

	err := future.Wait()

	assert.Equal(t, context.Canceled, err)
}
