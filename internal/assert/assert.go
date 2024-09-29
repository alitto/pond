package assert

import "testing"

/**
 * Asserts that the expected and actual values are equal.
 */
func Equal(t *testing.T, expected interface{}, actual interface{}) {
	if expected != actual {
		t.Helper()
		t.Errorf("Expected %T(%v) but was %T(%v)", expected, expected, actual, actual)
	}
}

/**
 * Asserts that the actual value is true.
 */
func True(t *testing.T, actual bool) {
	if !actual {
		t.Helper()
		t.Errorf("Expected true but was %T(%v)", actual, actual)
	}
}
