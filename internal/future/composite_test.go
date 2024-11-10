package future

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestCompositeFutureWait(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait(3)

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])
}

func TestCompositeFutureWaitWithError(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	sampleErr := errors.New("sample error")
	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", sampleErr)

	outputs, err := future.Wait(3)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "", outputs[2])
}

func TestCompositeFutureWaitWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	future, resolve := NewCompositeFuture[string](ctx)

	cancel()

	resolve(0, "output1", nil)

	_, err := future.Wait(1)

	assert.Equal(t, context.Canceled, err)
}

func TestCompositeFutureResolveWithIndexOutOfRange(t *testing.T) {
	_, resolve := NewCompositeFuture[string](context.Background())

	assert.PanicsWithError(t, "index must be greater than or equal to 0", func() {
		resolve(-1, "output1", nil)
	})
}

func TestCompositeFutureWithMultipleWait(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)

	outputs1, err := future.Wait(1)

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(outputs1))
	assert.Equal(t, "output1", outputs1[0])

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait(3)

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])
}

func TestCompositeFutureWithErrorsAndMultipleWait(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	sampleErr := errors.New("sample error")
	resolve(0, "output1", sampleErr)

	outputs1, err := future.Wait(1)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 1, len(outputs1))
	assert.Equal(t, "", outputs1[0])

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait(3)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "", outputs[0])
	assert.Equal(t, "", outputs[1])
	assert.Equal(t, "", outputs[2])
}

func TestCompositeFutureWaitBeforeResoluion(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve(0, "output1", nil)
	}()

	outputs, err := future.Wait(1)

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(outputs))
	assert.Equal(t, "output1", outputs[0])
}

func TestCompositeFutureWaitBeforeContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	future, _ := NewCompositeFuture[string](ctx)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	outputs, err := future.Wait(1)

	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, len(outputs))
	assert.Equal(t, "", outputs[0])
}

func TestCompositeFutureWaitWithContextCanceledAfterResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	future, resolve := NewCompositeFuture[string](ctx)

	resolve(0, "output1", nil)
	cancel()

	outputs, err := future.Wait(1)

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(outputs))
	assert.Equal(t, "output1", outputs[0])
}

func TestCompositeFutureWithMultipleDone(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)

	outputs1, err := future.Wait(1)
	<-future.Done(1)

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(outputs1))
	assert.Equal(t, "output1", outputs1[0])

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait(3)
	<-future.Done(3)

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])
}

func TestCompositeFutureWithErrorsAndMultipleDone(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	sampleErr := errors.New("sample error")
	resolve(0, "output1", sampleErr)

	outputs1, err := future.Wait(1)
	<-future.Done(1)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 1, len(outputs1))
	assert.Equal(t, "", outputs1[0])

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait(3)
	<-future.Done(3)

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 3, len(outputs))
}

func TestCompositeFutureCancel(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background())

	resolve(0, "output1", nil)

	sampleErr := errors.New("sample error")
	future.Cancel(sampleErr)

	outputs, err := future.Wait(1)

	assert.Equal(t, nil, err)
	assert.Equal(t, 1, len(outputs))
}
