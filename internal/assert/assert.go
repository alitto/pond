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

/**
 * Asserts that the function panics with the expected object.
 */
func PanicsWith(t *testing.T, expected any, f func()) {
	defer func() {
		if r := recover(); r != nil {
			Equal(t, expected, r)
		} else {
			t.Errorf("Expected a panic, but got nil")
		}
	}()
	f()
}

/**
 * Asserts that the function panics with the expected error.
 */
func PanicsWithError(t *testing.T, expected string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				t.Errorf("Expected a panic with error, but got %T(%v)", r, r)
				return
			}
			Equal(t, expected, err.Error())
		} else {
			t.Errorf("Expected a panic, but got nil")
		}
	}()
	f()
}
