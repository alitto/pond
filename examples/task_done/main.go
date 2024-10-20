package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/alitto/pond/v2"
)

func main() {

	pool := pond.NewResultPool[string](10)

	// Submit a task that runs for 0-300ms and returns "foo"
	task := pool.Submit(func() string {
		sleepTime := time.Duration((rand.IntN(4))*100) * time.Millisecond
		time.Sleep(sleepTime)
		return fmt.Sprintf("in %dms", sleepTime.Milliseconds())
	})

	// Wait for the task to complete or a 150ms timeout
	select {
	case <-task.Done():
		result, err := task.Wait()
		if err != nil {
			fmt.Printf("Task failed with error: %s\n", err)
			return
		} else {
			fmt.Printf("Task completed successfully %s\n", result)
		}
	case <-time.After(150 * time.Millisecond):
		fmt.Println("Task timed out")
	}
}
