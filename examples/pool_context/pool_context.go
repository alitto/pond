package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alitto/pond"
)

// Pressing Ctrl+C while this program is running will cause the program to terminate gracefully.
// Tasks being processed will continue until they finish, but queued tasks are cancelled.
func main() {

	// Create a context that will be cancelled when the user presses Ctrl+C (process receives termination signal).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Create a pool and pass the context to it.
	pool := pond.New(1, 1000, pond.Context(ctx))
	defer pool.StopAndWait()

	// Submit several long runnning tasks
	var count int = 100
	for i := 0; i < count; i++ {
		n := i
		pool.Submit(func() {
			fmt.Printf("Task #%d started\n", n)
			time.Sleep(1 * time.Second)
			fmt.Printf("Task #%d finished\n", n)
		})
	}
}
