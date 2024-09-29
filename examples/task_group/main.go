package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a task group
	group := pond.TypedGroup[string]()

	var urls = []string{
		"https://jsonplaceholder.typicode.com/todos/1",
		"https://jsonplaceholder.typicode.com/todos/2",
		"https://jsonplaceholder.typicode.com/todos/3",
	}

	// Submit tasks to fetch the contents of each URL
	for _, url := range urls {
		url := url
		group.Add(func() (string, error) {
			// Fetch the URL
			resp, err := http.Get(url)
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
	responseBodies, err := group.Get()

	if err != nil {
		fmt.Printf("Failed to fetch URLs: %v", err)
		return
	}

	for i, body := range responseBodies {
		fmt.Printf("URL %s: %s\n", urls[i], body)
	}
}
