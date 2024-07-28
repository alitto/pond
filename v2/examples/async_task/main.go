package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/alitto/pond/v2"
)

func main() {

	// Submit a sample task that runs asynchronously in the worker pool and returns a string
	task := pond.Submit[string](func() (string, error) {
		res, err := http.Get("https://jsonplaceholder.typicode.com/todos/1")
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

	// Submit the group and get the responses
	response, err := task.Get()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response)
}
