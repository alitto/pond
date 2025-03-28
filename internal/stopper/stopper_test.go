package stopper

import (
	"github.com/alitto/pond/v2/internal/assert"
	"testing"
)

func TestEmptyStopper(t *testing.T) {
	s := New()

	assert.False(t, s.Stopping())
	assert.False(t, s.Stopped())

	s.Stop()

	s.Add(1)

	assert.True(t, s.Stopping())
	assert.True(t, s.Stopped())
}

func TestStopper(t *testing.T) {
	s := New()

	assert.False(t, s.Stopping())
	assert.False(t, s.Stopped())

	s.Add(1)

	s.Stop()

	assert.True(t, s.Stopping())
	assert.False(t, s.Stopped())

	s.Done()

	assert.True(t, s.Stopping())
	assert.True(t, s.Stopped())
}
