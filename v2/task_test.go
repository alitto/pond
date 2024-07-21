package pond

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestVoidTaskSubmit(t *testing.T) {

	pool := NewPool(context.Background(), 10)

	task := NewTask(func(_ context.Context) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	}).WithPool(pool)

	err := task.Run().WaitErr()

	assertEqual(t, nil, err)

	pool.Stop().Wait()
}

func TestTaskSubmit(t *testing.T) {

	task := NewOTask[int](func(_ context.Context) (int, error) {
		time.Sleep(1 * time.Millisecond)
		return 5, nil
	})

	result, err := task.Run().Wait()

	assertEqual(t, 5, result)
	assertEqual(t, nil, err)
}

func TestTaskRun(t *testing.T) {

	task := NewOTask[int](func() int {
		time.Sleep(1 * time.Millisecond)
		return 5
	})

	result, err := task.Run().Wait()

	assertEqual(t, 5, result)
	assertEqual(t, nil, err)
}

func TestTaskRunWithContext(t *testing.T) {

	task := NewOTask[int](func(ctx context.Context) (int, error) {
		timer := time.NewTimer(1 * time.Millisecond)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timer.C:
		}
		return 5, nil
	})

	customCtx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	result, err := task.WithContext(customCtx).Run().Wait()

	assertEqual(t, 0, result)
	assertEqual(t, context.Canceled, err)
}

func TestIOTaskRun(t *testing.T) {

	squaredTask := NewIOTask[float64, float64](func(input float64) float64 {
		return math.Pow(input, 2)
	})

	result, err := squaredTask.WithInput(10).Run().Wait()

	assertEqual(t, float64(100), result)
	assertEqual(t, nil, err)
}

func TestIOTaskGroupRun(t *testing.T) {

	squaredTask := NewIOTask[float64, float64](func(input float64) float64 {
		return math.Pow(input, 2)
	})

	result, err := squaredTask.WithInputs(1, 2, 3, 4, 5).Run().Wait()

	assertEqual(t, float64(1), result[0])
	assertEqual(t, float64(4), result[1])
	assertEqual(t, float64(9), result[2])
	assertEqual(t, float64(16), result[3])
	assertEqual(t, float64(25), result[4])
	assertEqual(t, nil, err)
}
