package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alitto/pond/v2"
)

// Pressing Ctrl+C while this program is running will cause the program to terminate gracefully.
// Tasks being processed will continue until they finish, but queued tasks are cancelled.
func main() {

	// Create a context that will be cancelled when the user presses Ctrl+C (process receives termination signal).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		fmt.Println("Ctrl+C pressed, stopping the pool...")
	}()

	// Create a pool and pass the context to it.
	pool := pond.NewPool(10, pond.WithContext(ctx))

	// Submit several long running tasks
	var count int = 100
	for i := 0; i < count; i++ {
		i := i
		pool.Submit(func() {
			fmt.Printf("Task #%d started\n", i)
			time.Sleep(3 * time.Second)
			fmt.Printf("Task #%d finished\n", i)
		})
	}

	// Stop the pool and wait for all running tasks to finish
	err := pool.Stop().Wait()
	if err != nil {
		fmt.Printf("Pool stopped with error: %v\n", err)
	} else {
		fmt.Println("Pool stopped gracefully")
	}
}
