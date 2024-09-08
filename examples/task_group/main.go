package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a worker pool
	pool := pond.NewTypedPool[string](1000)
	defer pool.Stop()

	// Create a task group
	group := pool.Group()

	var urls = []string{
		"https://jsonplaceholder.typicode.com/todos/1",
		"https://jsonplaceholder.typicode.com/todos/2",
		"https://jsonplaceholder.typicode.com/todos/3",
	}

	ctx := context.Background()

	// Submit tasks to fetch the contents of each URL
	for _, url := range urls {
		url := url
		group.Add(func() (string, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return "", err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()

			// Get response body as a string
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", err
			}

			return string(body), nil
		})
	}

	// Wait for all HTTP requests to complete.
	responseBodies, err := group.Submit().Get()

	if err != nil {
		fmt.Printf("Failed to fetch URLs: %v", err)
		return
	}

	for i, body := range responseBodies {
		fmt.Printf("URL %s: %s\n", urls[i], body)
	}
}
