package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a new result pool
	pool := pond.NewResultPool[time.Duration](0)

	// Submit a task that runs for 1-3 seconds and returns the sleep time
	task := pool.Submit(func() time.Duration {
		sleepTime := time.Duration(1+rand.IntN(3)) * time.Second
		time.Sleep(time.Duration(sleepTime))
		return sleepTime
	})

	// Submit the task and wait for it to complete
	sleepTime, err := task.Wait()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Task completed successfully in %f seconds\n", sleepTime.Seconds())
}
