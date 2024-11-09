package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
)

func main() {
	// Generate 1000 tasks that each take 1 second to complete
	tasks := generateTasks(1000, 1*time.Second)

	// Create a pool with a max concurrency of 10
	pool := pond.NewPool(10)
	defer pool.StopAndWait()

	// Create a context with a timeout of 5 seconds
	timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a group with the timeout context
	group := pool.NewGroupContext(timeout)

	// Submit all tasks to the group and wait for them to complete or the timeout to expire
	err := group.Submit(tasks...).Wait()

	if err != nil {
		fmt.Printf("Group completed with error: %v\n", err)
	} else {
		fmt.Println("Group completed successfully")
	}
}

func generateTasks(count int, duration time.Duration) []func() {

	tasks := make([]func(), count)

	for i := 0; i < count; i++ {
		i := i
		tasks[i] = func() {
			time.Sleep(duration)
			fmt.Printf("Task #%d finished\n", i)
		}
	}

	return tasks
}
