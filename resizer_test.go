package pond

import (
	"testing"
)

func TestRatedResizer(t *testing.T) {

	resizer := RatedResizer(3)

	assertEqual(t, true, resizer.Resize(0, 0, 10))
	assertEqual(t, true, resizer.Resize(0, 0, 10))
	assertEqual(t, true, resizer.Resize(1, 0, 10))
	assertEqual(t, false, resizer.Resize(2, 0, 10))
	assertEqual(t, false, resizer.Resize(3, 0, 10))
	assertEqual(t, true, resizer.Resize(4, 0, 10))
}

func TestRatedResizerWithRate1(t *testing.T) {

	resizer := RatedResizer(1)

	assertEqual(t, true, resizer.Resize(0, 0, 10))
	assertEqual(t, true, resizer.Resize(1, 0, 10))
	assertEqual(t, true, resizer.Resize(2, 0, 10))
}

func TestRatedResizerWithInvalidRate(t *testing.T) {

	resizer := RatedResizer(0)

	assertEqual(t, true, resizer.Resize(0, 0, 10))
	assertEqual(t, true, resizer.Resize(1, 0, 10))
	assertEqual(t, true, resizer.Resize(2, 0, 10))
}

func TestPresetRatedResizers(t *testing.T) {

	eager := Eager()
	balanced := Balanced()
	lazy := Lazy()

	assertEqual(t, true, eager.Resize(0, 0, 10))
	assertEqual(t, true, balanced.Resize(0, 0, 10))
	assertEqual(t, true, lazy.Resize(0, 0, 10))
}
