package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alitto/pond"
)

func main() {

	// Create a worker pool
	pool := pond.New(10, 1000)
	defer pool.StopAndWait()

	// Create a task group associated to a context
	group, ctx := pool.GroupContext(context.Background())

	var urls = []string{
		"https://www.golang.org/",
		"https://www.google.com/",
		"https://www.github.com/",
	}

	// Submit tasks to fetch each URL
	for _, url := range urls {
		url := url
		group.Submit(func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
			return err
		})
	}

	// Wait for all HTTP requests to complete.
	err := group.Wait()
	if err != nil {
		fmt.Printf("Failed to fetch URLs: %v", err)
	} else {
		fmt.Println("Successfully fetched all URLs")
	}
}
