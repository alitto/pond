package future

import (
	"context"
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestCompositeFuture(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background(), 3)

	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", nil)

	outputs, err := future.Get()

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(outputs))
	assert.Equal(t, "output1", outputs[0])
	assert.Equal(t, "output2", outputs[1])
	assert.Equal(t, "output3", outputs[2])

	err = future.Wait()

	assert.Equal(t, nil, err)
}

func TestCompositeFutureWithError(t *testing.T) {
	future, resolve := NewCompositeFuture[string](context.Background(), 3)

	sampleErr := errors.New("sample error")
	resolve(0, "output1", nil)
	resolve(1, "output2", nil)
	resolve(2, "output3", sampleErr)

	outputs, err := future.Get()

	assert.Equal(t, sampleErr, err)
	assert.Equal(t, 0, len(outputs))

	err = future.Wait()

	assert.Equal(t, sampleErr, err)
}

func TestCompositeFutureWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	future, resolve := NewCompositeFuture[string](ctx, 2)

	cancel()

	resolve(0, "output1", nil)

	_, err := future.Get()

	assert.Equal(t, context.Canceled, err)

	err = future.Wait()

	assert.Equal(t, context.Canceled, err)
}
