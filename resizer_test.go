package pond

import (
	"testing"
	"time"
)

func TestResize(t *testing.T) {

	resizer := DynamicResizer(3, 0.1)

	// First resize should grow the pool proportionally
	assertEqual(t, 10, resizer.Resize(0, 0, 1, 100, 10, 0, 1*time.Second))

	// Now the input rate grows but below the tolerance (10%)
	assertEqual(t, -1, resizer.Resize(10, 10, 1, 100, 1, 10, 1*time.Second))

	// Now the input rate grows more
	assertEqual(t, 90, resizer.Resize(10, 10, 1, 100, 100000, 11, 1*time.Second))

	// Now there's no new tasks for 3 cycles
	assertEqual(t, -1, resizer.Resize(10, 10, 1, 100, 0, 100011, 1*time.Second))
	assertEqual(t, -1, resizer.Resize(10, 10, 1, 100, 0, 100011, 1*time.Second))
	assertEqual(t, 0, resizer.Resize(1, 1, 1, 100, 0, 100011, 10*time.Second))
}

func TestEagerPool(t *testing.T) {
	pool := New(100, 1000, Strategy(Eager()))
	pool.debug = true

	for i := 0; i < 100; i++ {
		pool.Submit(func() {
			time.Sleep(1 * time.Millisecond)
		})
	}

	pool.StopAndWait()

	assertEqual(t, 100, pool.maxWorkerCount)
}
