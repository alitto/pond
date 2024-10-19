package future

import (
	"context"
	"errors"
	"testing"
	"time"

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
	assert.Equal(t, sampleErr, future.Err())
}

func TestFutureWaitWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := NewFuture(ctx)

	cancel()

	resolve(errors.New("sample error"))

	err := future.Wait()

	assert.Equal(t, context.Canceled, err)
}

func TestFutureDone(t *testing.T) {

	ctx := context.Background()

	future, resolve := NewFuture(ctx)

	sampleErr := errors.New("sample error")

	go func() {
		time.Sleep(1 * time.Millisecond)
		resolve(sampleErr)
	}()

	<-future.Done()

	assert.Equal(t, sampleErr, future.Err())
}

func TestFutureResolutionError(t *testing.T) {

	resolution := &futureResolution{
		err: errors.New("sample error"),
	}

	assert.Equal(t, "sample error", resolution.Error())

	resolution = &futureResolution{}

	assert.Equal(t, "future resolved", resolution.Error())
}
