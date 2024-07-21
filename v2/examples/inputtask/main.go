package main

import (
	"fmt"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a buffered (non-blocking) pool that can scale up to 100 workers
	// and has a buffer capacity of 1000 tasks
	task := pond.NewITask[int](func(n int) {
		fmt.Printf("Running task #%d\n", n)
	}).WithMaxConcurrency(1)

	// Submit 1000 tasks
	for i := 0; i < 1000; i++ {
		task.WithInput(i).Run()
	}

	// Stop the pool and wait for all submitted tasks to complete
	task.Pool().Stop().Wait()
}
