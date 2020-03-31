<a title="Build Status" target="_blank" href="https://travis-ci.com/alitto/pond"><img src="https://travis-ci.com/alitto/pond.svg?branch=master&status=passed"></a>
<a title="Codecov" target="_blank" href="https://codecov.io/gh/alitto/pond"><img src="https://codecov.io/gh/alitto/pond/branch/master/graph/badge.svg"></a>
<a title="Release" target="_blank" href="https://github.com/alitto/pond/releases"><img src="https://img.shields.io/github/v/release/alitto/pond"></a>
<a title="Go Report Card" target="_blank" href="https://goreportcard.com/report/github.com/alitto/pond"><img src="https://goreportcard.com/badge/github.com/alitto/pond"></a>

# pond
Minimalistic and High-performance goroutine worker pool written in Go

## Motivation

This library is meant to provide a simple way to limit concurrency when executing some function over a limited resource or service.

Some common scenarios include:

 - Executing queries against a Database with a limited no. of connections
 - Sending HTTP requests to a a rate/concurrency limited API

## Features:

- Zero dependencies
- Create pools with fixed or dynamic size
- Worker goroutines are only created when needed (backpressure detection) and automatically purged after being idle for some time (configurable)
- Minimalistic APIs for:
  - Creating worker pools with fixed or dynamic size
  - Submitting tasks to a pool in a fire-and-forget fashion
  - Submitting tasks to a pool and waiting for them to complete
  - Submitting tasks to a pool with a deadline
  - Submitting a group of related tasks and waiting for them to complete
  - Getting the number of running workers (goroutines)
  - Stopping a worker pool
- Task panics are handled gracefully (configurable panic handler)
- Supports Non-blocking and Blocking task submission modes (buffered / unbuffered)
- Very high performance under heavy workloads (See [benchmarks](#benchmarks))
- [API reference](https://pkg.go.dev/github.com/alitto/pond)

## How to install

```powershell
go get -u github.com/alitto/pond
```

## How to use

### Worker pool with dynamic size

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond"
)

func main() {

	// Create a buffered (non-blocking) pool that can scale up to 100 workers
	// and has a buffer capacity of 1000 tasks
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

### Worker pool with fixed size

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond"
)

func main() {

	// Create an unbuffered (blocking) pool with a fixed 
	// number of workers
	pool := pond.New(10, 0, pond.MinWorkers(10))

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

### Submitting groups of related tasks

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond"
)

func main() {

	// Create a pool
	pool := pond.New(10, 1000)
	defer pool.StopAndWait()

	// Create a task group
	group := pool.Group()

	// Submit a group of related tasks
	for i := 0; i < 20; i++ {
		n := i
		group.Submit(func() {
			fmt.Printf("Running group task #%d\n", n)
		})
	}

	// Wait for all tasks in the group to complete
	group.Wait()
}
```

### Pool Configuration Options

- **MinWorkers**: Specifies the minimum number of worker goroutines that must be running at any given time. These goroutines are started when the pool is created. Example:
``` go
// This will create a pool with 5 running worker goroutines 
pool := pond.New(10, 1000, pond.MinWorkers(5))
```
- **IdleTimeout**: Defines how long to wait before removing idle worker goroutines from the pool. Example:
``` go
// This will create a pool that will remove workers 100ms after they become idle 
pool := pond.New(10, 1000, pond.IdleTimeout(100 * time.Millisecond))
``` 
- **PanicHandler**: Allows to configure a custom function to handle panics thrown by tasks submitted to the pool. Example:
```go
// Custom panic handler function
panicHandler := func(p interface{}) {
	fmt.Printf("Task panicked: %v", p)
}

// This will create a pool that will handle panics using a custom panic handler
pool := pond.New(10, 1000, pond.PanicHandler(panicHandler)))
``` 

## API Reference

Full API reference is available at https://pkg.go.dev/github.com/alitto/pond

## Benchmarks

We ran a few [benchmarks](benchmark/benchmark_test.go) to show how _pond_'s performance compares against some of the most popular worker pool libraries available for Go ([ants](https://github.com/panjf2000/ants/) and [gammazero's workerpool](https://github.com/gammazero/workerpool)).

We also included benchmarks to compare it against just launching 1M goroutines and manually creating a goroutine worker pool (inspired by [gobyexample.com](https://gobyexample.com/worker-pools)), using either a buffered or an unbuffered channel to dispatch tasks. 

The test consists of submitting 1 million tasks to the pool, each of them simulating a 10ms operation by executing `time.Sleep(10 * time.Millisecond)`. All pools are configured to use a maximum of 200k workers and initialization times are not taken into account.

Here are the results:

```powershell
goos: linux
goarch: amd64
pkg: github.com/alitto/pond/benchmark
BenchmarkPond-8                    	       2	 503513856 ns/op	65578500 B/op	 1057273 allocs/op
BenchmarkGoroutines-8              	       3	 444264750 ns/op	81560042 B/op	 1003312 allocs/op
BenchmarkGoroutinePool-8           	       1	1035752534 ns/op	79889952 B/op	  512480 allocs/op
BenchmarkBufferedGoroutinePool-8   	       2	 968502858 ns/op	51945376 B/op	  419122 allocs/op
BenchmarkGammazeroWorkerpool-8     	       1	1413724148 ns/op	18018800 B/op	 1023746 allocs/op
BenchmarkAnts-8                    	       2	 665947820 ns/op	19401172 B/op	 1046906 allocs/op
PASS
ok  	github.com/alitto/pond/benchmark	12.109s
Success: Benchmarks passed.
```

As you can see, _pond_ (503.5ms) outperforms _ants_ (665.9ms), _Gammazero's workerpool_ (1413.7ms), unbuffered goruotine pool (1035.8ms) and buffered goroutine pool (968.5ms) but it falls behind unlimited goroutines (444.3ms).

These tests were executed on a laptop with an 8-core CPU (Intel(R) Core(TM) i7-8550U CPU @ 1.80GHz) and 16GB of RAM.

## Resources

Here are some of the resources which have served as inspiration when writing this library:

- http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/
- https://brandur.org/go-worker-pool
- https://gobyexample.com/worker-pools
- https://github.com/panjf2000/ants
- https://github.com/gammazero/workerpool

## Contribution & Support

Feel free to send a pull request if you consider there's something which can be improved. Also, please open up an issue if you run into a problem when using this library or just have a question.