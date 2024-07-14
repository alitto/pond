package pondv4

import (
	"context"
	"testing"
)

func TestGenericPool(t *testing.T) {

	pool := NewGenericPool(context.Background(), 1)

	taskCtx := pool.Submit(func() (any, error) {
		return 5, nil
	})

	result, err := WaitFor[int](taskCtx)

	assertEqual(t, 5, result)
	assertEqual(t, nil, err)
}

func TestGenericPool2(t *testing.T) {

	pool := NewGenericPool(context.Background(), 1)

	result, err := WaitFor[int](pool.Submit(func() (any, error) {
		return 5, nil
	}))

	assertEqual(t, 5, result)
	assertEqual(t, nil, err)
}
