# pond
pond: Minimalistic and High-performance goroutine worker pool written in Go

## Features:

- Managing and recycling a massive number of goroutines automatically
- Purging overdue goroutines periodically
- Minimalistic API for submitting tasks, getting the number of running goroutines, stopping the pool and more.
- Zero dependencies
- Handle task panics gracefully
- Efficient memory usage
- Blocking and Nonblocking modes supported

## How to install

```powershell
go get -u github.com/alitto/pond
```

## How to use

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond"
)

func main() {

	// Create a pool with 100 workers
	pool := pond.New(100, 1000)

	// Submit 1000 tasks
	for i := 0; i < 1000; i++ {
		n := i
		pool.Submit(func() {
			fmt.Printf("Running task #%d\n", n)
		})
	}

	// Stop the pool and wait for all submitted tasks to complete
	pool.StopAndWait()
}
```