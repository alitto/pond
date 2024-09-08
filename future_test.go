package pond

import (
	"context"
	"fmt"
	"testing"
)

func TestFutureGet(t *testing.T) {

	ctx := context.Background()

	future, resolve := newFuture[int](ctx)

	resolve(5, nil)

	out, err := future.Get()

	assertEqual(t, nil, err)
	assertEqual(t, 5, out)
}

func TestFutureGetWithError(t *testing.T) {

	ctx := context.Background()

	future, resolve := newFuture[int](ctx)

	simpleErr := fmt.Errorf("error")

	resolve(0, simpleErr)

	out, err := future.Get()

	assertEqual(t, simpleErr, err)
	assertEqual(t, "error", err.Error())
	assertEqual(t, 0, out)
}

func TestFutureGetWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := newFuture[int](ctx)

	cancel()

	resolve(0, nil)

	out, err := future.Get()

	assertEqual(t, context.Canceled, err)
	assertEqual(t, 0, out)
}

func TestFutureWait(t *testing.T) {

	ctx := context.Background()

	future, resolve := newFuture[int](ctx)

	resolve(5, nil)

	err := future.Wait()

	assertEqual(t, nil, err)
}

func TestFutureWaitWithError(t *testing.T) {

	ctx := context.Background()

	future, resolve := newFuture[int](ctx)

	resolve(0, fmt.Errorf("error"))

	err := future.Wait()

	assertEqual(t, "error", err.Error())
}

func TestFutureWaitWithCanceledContext(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())

	future, resolve := newFuture[int](ctx)

	cancel()

	resolve(0, nil)

	err := future.Wait()

	assertEqual(t, context.Canceled, err)
}

func TestMap(t *testing.T) {

	pool := NewTypedPool[int](10)

	task := pool.Submit(func() int {
		return 2
	})

	formatted, err := Map(task, func(in int) string {
		return fmt.Sprintf("number is %d", in)
	}).Get()

	assertEqual(t, nil, err)
	assertEqual(t, "number is 2", formatted)
}

func TestMapWithError(t *testing.T) {

	pool := NewTypedPool[int](10)

	task := pool.Submit(func() (int, error) {
		return 0, fmt.Errorf("error")
	})

	formatted, err := Map(task, func(in int) string {
		return fmt.Sprintf("number is %d", in)
	}).Get()

	assertEqual(t, "error", err.Error())
	assertEqual(t, "", formatted)
}

func TestFutureResultError(t *testing.T) {
	result := &futureResult[int]{
		output: 0,
		err:    fmt.Errorf("error"),
	}

	assertEqual(t, "error", result.Error())

	resultOk := &futureResult[string]{
		output: "hello",
		err:    nil,
	}

	assertEqual(t, "result: \"hello\"", resultOk.Error())
}
