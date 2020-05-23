package pond

import (
	"testing"
	"time"
)

func assertEqual(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

func TestNew(t *testing.T) {

	pool := New(17, 10, MinWorkers(2), IdleTimeout(1*time.Second))
	assertEqual(t, 17, pool.maxWorkers)
	assertEqual(t, 10, pool.maxCapacity)
	assertEqual(t, 2, pool.minWorkers)
	assertEqual(t, 1*time.Second, pool.idleTimeout)
}

func TestNewWithInconsistentOptions(t *testing.T) {

	pool := New(-10, -5, MinWorkers(20), IdleTimeout(-1*time.Second))
	assertEqual(t, 1, pool.maxWorkers)
	assertEqual(t, 0, pool.maxCapacity)
	assertEqual(t, 1, pool.minWorkers)
	assertEqual(t, defaultIdleTimeout, pool.idleTimeout)
}
