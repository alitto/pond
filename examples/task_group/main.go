package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/alitto/pond/v2"
)

func fetchURL(ctx context.Context, url string) (string, error) {
	// Create a new HTTP request with the provided URL and context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return "", err
	}

	// Send the HTTP request
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Get response body as a string
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

var urls = []string{
	"https://jsonplaceholder.typicode.com/todos/1",
	"https://jsonplaceholder.typicode.com/todos/2",
	"https://jsonplaceholder.typicode.com/todos/3",
}

func main() {

	ctx := context.Background()

	// Create a pool with a result type of string
	pool := pond.NewResultPool[string](10)

	// Create a task group
	group := pool.NewGroup()

	// Create tasks for fetching URLs
	for _, url := range urls {
		url := url
		group.SubmitErr(func() (string, error) {
			return fetchURL(ctx, url)
		})
	}

	// Wait for all HTTP requests to complete.
	responses, err := group.Wait()

	if err != nil {
		fmt.Printf("Failed to fetch URLs: %v", err)
		return
	}

	for i, res := range responses {
		fmt.Printf("URL %s: %s\n", urls[i], res)
	}
}
