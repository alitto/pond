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
	future, resolve := NewCompositeFuture[string](context.Background(), 3)

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

	var thrownPanic interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				thrownPanic = r
			}
		}()
		resolve(2, "output1", nil)
	}()

	assert.True(t, thrownPanic != nil)
	assert.Equal(t, "index must be less than 2", thrownPanic.(error).Error())

	var thrownPanic2 interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				thrownPanic2 = r
			}
		}()
		resolve(-1, "output2", nil)
	}()

	assert.True(t, thrownPanic2 != nil)
	assert.Equal(t, "index must be greater than or equal to 0", thrownPanic2.(error).Error())
}
