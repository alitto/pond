package future

import (
	"context"
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestCompositeFutureWait(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background(), 3)

	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])
}

func TestCompositeFutureWaitWithError(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background(), 1)
	future.Add(2)

	sampleErr := errors.New("sample error")
	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", sampleErr)

	outputs, err := future.Wait()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 0, len(outputs))
}

func TestCompositeFutureWaitWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	future, resolve := NewCompositeFuture[string](ctx, 2)

	cancel()

	resolve(0, "output1", nil)

	_, err := future.Wait()

	assert.Equal(t, context.Canceled, err)
}

func TestCompositeFutureResolveWithIndexOutOfRange(t *testing.T) {
	_, resolve := NewCompositeFuture[string](context.Background(), 2)

	assert.PanicsWithError(t, "index must be less than 2", func() {
		resolve(2, "output1", nil)
	})

	assert.PanicsWithError(t, "index must be greater than or equal to 0", func() {
		resolve(-1, "output1", nil)
	})
}

func TestCompositeFutureAddWithInvalidCount(t *testing.T) {
	future, _ := NewCompositeFuture[string](context.Background(), 2)

	assert.PanicsWithError(t, "delta must be greater than 0", func() {
		future.Add(0)
	})
}

func TestCompositeFutureAdd(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background(), 2)

	resolve(0, "output1", nil)

	future.Add(1)

	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Wait()

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])
}
