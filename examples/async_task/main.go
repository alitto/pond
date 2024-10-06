package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/alitto/pond/v2"
)

func main() {

	pool := pond.WithOutput[time.Duration]()

	// Submit a sample task that runs asynchronously in the worker pool and returns a string
	task := pool.Submit(func() time.Duration {
		sleepTime := time.Duration(1+rand.IntN(3)) * time.Second
		time.Sleep(time.Duration(sleepTime))
		return sleepTime
	})

	// Submit the group and get the responses
	sleepTime, err := task.Get()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Task completed successfully in %f seconds\n", sleepTime.Seconds())
}
