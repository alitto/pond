package pondv4

import (
	"context"
	"testing"
)

func TestFuncPool(t *testing.T) {

	pool := NewFuncPool[int, int](context.Background(), 1, func(input int) int {
		return input * 2
	})

	result, err := pool.Invoke(3).Wait()

	pool.StopAndWait()

	assertEqual(t, 6, result)
	assertEqual(t, nil, err)
}
