<a title="Codecov" target="_blank" href="https://github.com/alitto/pond/actions"><img alt="Build status" src="https://github.com/alitto/pond/actions/workflows/main.yml/badge.svg?branch=main&event=push"/></a>
<a title="Codecov" target="_blank" href="https://codecov.io/gh/alitto/pond"><img src="https://codecov.io/gh/alitto/pond/branch/main/graph/badge.svg"/></a>
<a title="Release" target="_blank" href="https://github.com/alitto/pond/releases"><img src="https://img.shields.io/github/v/release/alitto/pond"/></a>
<a title="Go Report Card" target="_blank" href="https://goreportcard.com/report/github.com/alitto/pond"><img src="https://goreportcard.com/badge/github.com/alitto/pond"/></a>

# pond
A minimalistic and high-performance Go library designed to elegantly manage concurrent tasks.

## Motivation

This library is meant to provide a simple and idiomatic way to manage concurrency in golang programs. Based on the [Worker Pool pattern](https://en.wikipedia.org/wiki/Thread_pool), it allows you to limit the number of goroutines running concurrently and queue tasks when the pool is full. This is useful when you need to limit the number of concurrent operations to avoid resource exhaustion or rate limiting.  

Some common use cases include:
 - Processing a large number of tasks concurrently
 - Limiting the number of concurrent HTTP requests
 - Limiting the number of concurrent database connections
 - Sending HTTP requests to a rate-limited API

## Features:

- Zero dependencies
- Create pools with maximum number of workers
- Worker goroutines are only created when needed and immediately removed when idle (scale to zero by default)
- Minimalistic and fluent APIs for:
  - Creating worker pools with maximum number of workers
  - Submitting tasks to a pool in a fire-and-forget fashion
  - Submitting tasks to a pool and waiting for them to complete
  - Submitting a group of tasks and waiting for them to complete or the first error to occur
  - Submitting a group of tasks and waiting for all of them to complete successfully or with an error 
  - Stopping a worker pool
  - Monitoring pool metrics such as number of running workers, tasks waiting in the queue, etc.
- Very high performance and efficient resource usage under heavy workloads, even outperforming unbounded goroutines in some scenarios
- Complete pool metrics such as number of running workers, tasks waiting in the queue [and more](#metrics--monitoring)
- Configurable parent context to stop all workers when it is cancelled
- **New features in v2**:
  - Unbounded task queues
  - Submission of typed tasks that return a value or an error
  - Awaitable task completion
  - Task panics are returned as errors
- [API reference](https://pkg.go.dev/github.com/alitto/pond)

## Installation

```bash
go get -u github.com/alitto/pond/v2
```

## Usage

### Submitting tasks to a pool with limited concurrency (maximum number of workers)

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a pool that can scale up to 100 workers
	pool := pond.NewPool(100)

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

### Submitting tasks that return a value

This feature allows you to submit tasks that return a value. This is useful when you need to process the result of a task.

``` go
// Create a pool that can scale up to 10 workers and accepts tasks that return a string or an error
pool := pond.WithOutput[string]().NewPool(10)

// Submit a task that returns a string value or an error
task := pool.Submit(func() (string) {
	return "Hello, World!"
})

// Wait for the task to complete and get the result
result, err := task.Get()

if err != nil {
	fmt.Printf("Failed to run task: %v", err)
} else {
	fmt.Printf("Task result: %v", result)
}
```

### Submitting tasks that return a value or an error

This feature allows you to submit tasks that return a value or an error. This is useful when you need to handle errors that occur during the execution of a task.

``` go
// Create a pool that can scale up to 10 workers and accepts tasks that return a string or an error
pool := pond.WithOutput[string]().NewPool(10)

// Submit a task that returns a string value or an error
task := pool.SubmitErr(func() (string, error) {
	return "Hello, World!", nil
})

// Wait for the task to complete and get the result
result, err := task.Get()

if err != nil {
	fmt.Printf("Failed to run task: %v", err)
} else {
	fmt.Printf("Task result: %v", result)
}
```

### Submitting a group of related tasks

``` go
// Create a pool that can scale up to 10 workers
pool := pond.NewPool(10)

// Create a task group
group := pool.Group()

// Submit a group of tasks
for i := 0; i < 20; i++ {
	n := i
	group.Submit(func() {
		fmt.Printf("Running group task #%d\n", n)
	})
}

// Wait for all tasks in the group to complete or the first error to occur
err := group.Wait()
if err != nil {
	fmt.Printf("Failed to complete group tasks: %v", err)
} else {
	fmt.Println("Successfully completed all group tasks")
}
```

Alternatively, you can wait for all tasks in the group to complete and get all errors that occurred during their execution:

``` go
// Wait for all tasks in the group to complete and get all errors
results, err := group.All()

if err != nil {
	fmt.Printf("Failed to complete group tasks")
	for i, err := range results {
		if err != nil {
			fmt.Printf("Task #%d failed: %v", i, err)
		}
	}
} else {
	fmt.Println("Successfully completed all group tasks")
}
```

### Submitting a group of tasks associated to a context

This feature provides synchronization, error propagation, and Context cancelation for a group of related tasks. Similar to `errgroup.Group` from [`golang.org/x/sync/errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup) package but with the concurrency bounded by the worker pool.

``` go
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a worker pool
	pool := pond.NewPool(1000)
	defer pool.StopAndWait()

	// Create a context
	ctx := context.Background()

	// Create a task group
	group := pool.Group()

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
			if err != nil {
				return err
			}
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
```

### Using a Custom Context

By default, all worker pools are associated to the `context.Background()` context, but you can associate a pool to a custom context by using the `pond.WithContext` function. This is useful when you need to be able to stop all running workers when a parent context is cancelled.

```go
// Create a custom context that can be cancelled
customCtx, cancel := context.WithCancel(context.Background())

// This creates a pool that is stopped when customCtx is cancelled 
pool := pond.WithContext(customCtx).NewPool(10)
``` 

### Stopping a pool

There are 3 methods available to stop a pool and release associated resources:
- `pool.Stop()`: stop accepting new tasks and signal all workers to stop processing new tasks. Tasks being processed by workers will continue until completion unless the process is terminated.
- `pool.StopAndWait()`: stop accepting new tasks and wait until all running and queued tasks have completed before returning.

### Metrics & monitoring

Each worker pool instance exposes useful metrics that can be queried through the following methods:

- `pool.RunningWorkers() int64`: Current number of running workers
- `pool.SubmittedTasks() uint64`: Total number of tasks submitted since the pool was created
- `pool.WaitingTasks() uint64`: Current number of tasks in the queue that are waiting to be executed
- `pool.SuccessfulTasks() uint64`: Total number of tasks that have successfully completed their exection since the pool was created
- `pool.FailedTasks() uint64`: Total number of tasks that completed with panic since the pool was created
- `pool.CompletedTasks() uint64`: Total number of tasks that have completed their exection either successfully or with panic since the pool was created

In our [Prometheus example](./examples/prometheus/main.go) we showcase how to configure collectors for these metrics and expose them to Prometheus.

## Examples

- [Creating a worker pool with dynamic size](./examples/dynamic_size/dynamic_size.go)
- [Creating a worker pool with fixed size](./examples/fixed_size/fixed_size.go)
- [Creating a worker pool with a Context](./examples/pool_context/pool_context.go)
- [Exporting worker pool metrics to Prometheus](./examples/prometheus/prometheus.go)
- [Submitting a group of tasks](./examples/task_group/task_group.go)
- [Submitting a group of tasks associated to a context](./examples/group_context/group_context.go)

## API Reference

Full API reference is available at https://pkg.go.dev/github.com/alitto/pond

## Benchmarks

See [Benchmarks](https://github.com/alitto/pond-benchmarks).

## Resources

Here are some of the resources which have served as inspiration when writing this library:

- http://marcio.io/2015/07/handling-1-million-requests-per-minute-with-golang/
- https://brandur.org/go-worker-pool
- https://gobyexample.com/worker-pools
- https://github.com/panjf2000/ants
- https://github.com/gammazero/workerpool

## Contribution & Support

Feel free to send a pull request if you consider there's something that should be polished or improved. Also, please open up an issue if you run into a problem when using this library or just have a question about it.