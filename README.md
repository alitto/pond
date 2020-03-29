<a title="Build Status" target="_blank" href="https://travis-ci.com/alitto/pond"><img src="https://travis-ci.com/alitto/pond.svg?branch=master&status=passed"></a>
<a title="Codecov" target="_blank" href="https://codecov.io/gh/alitto/pond"><img src="https://codecov.io/gh/alitto/pond/branch/master/graph/badge.svg"></a>
<a title="Release" target="_blank" href="https://github.com/alitto/pond/releases"><img src="https://img.shields.io/github/v/release/alitto/pond"></a>
<a title="Go Report Card" target="_blank" href="https://goreportcard.com/report/github.com/alitto/pond"><img src="https://goreportcard.com/badge/github.com/alitto/pond"></a>

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