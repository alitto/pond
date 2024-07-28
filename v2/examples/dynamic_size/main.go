package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a new pool of up to 10 workers
	pool := pond.NewPool[string](context.Background(), 10)

	// Create a new group
	group := pool.Group()

	// Add 10 tasks to the group
	for i := 0; i < 10; i++ {
		group.Add(func() (string, error) {
			res, err := http.Get(fmt.Sprintf("https://jsonplaceholder.typicode.com/todos/%d", i+1))
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			// read body
			resBody, err := io.ReadAll(res.Body)
			if err != nil {
				log.Fatalf("impossible to read all body of response: %s", err)
			}
			return string(resBody), err
		})
	}

	// Submit the group and get the responses
	responses, err := group.Submit().Get()

	if err != nil {
		panic(err)
	}

	for i, res := range responses {
		fmt.Printf("Response %d: %s\n", i, res)
	}
}
