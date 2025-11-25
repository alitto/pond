package pond

import (
	"errors"
	"testing"

	"github.com/alitto/pond/v2/internal/assert"
)

func TestInvokeTaskHandlesPanicByDefault(t *testing.T) {
	sampleErr := errors.New("sample error")

	err := func() (err error) {
		_, err = invokeTask[struct{}](func() {
			panic(sampleErr)
		}, true)
		return
	}()

	assert.True(t, errors.Is(err, ErrPanic))
	assert.True(t, errors.Is(err, sampleErr))
}

func TestInvokeTaskWithoutPanicRecoveryPanics(t *testing.T) {
	assert.PanicsWith(t, "boom", func() {
		invokeTask[struct{}](func() {
			panic("boom")
		}, false)
	})
}
