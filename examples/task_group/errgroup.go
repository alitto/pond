package main

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

func conventional() {

	var group errgroup.Group
	ctx := context.Background()
	responses := make([]string, len(urls))

	for _, url := range urls {
		url := url
		group.Go(func() error {
			// Fetch the URL.
			res, err := fetchURL(ctx, url)

			if err != nil {
				return err
			}

			responses = append(responses, res)

			return nil
		})
	}

	// Wait for all HTTP fetches to complete.
	if err := group.Wait(); err == nil {
		fmt.Printf("Failed to fetch URLs: %v", err)
		return
	}

	for i, res := range responses {
		fmt.Printf("URL %s: %s\n", urls[i], res)
	}
}
