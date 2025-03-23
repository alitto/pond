<a title="Codecov" target="_blank" href="https://github.com/alitto/pond/actions"><img alt="Build status" src="https://github.com/alitto/pond/actions/workflows/main.yml/badge.svg?branch=main&event=push"/></a>
<a title="Codecov" target="_blank" href="https://app.codecov.io/gh/alitto/pond/tree/main"><img src="https://codecov.io/gh/alitto/pond/branch/main/graph/badge.svg"/></a>
<a title="Release" target="_blank" href="https://github.com/alitto/pond/releases"><img src="https://img.shields.io/github/v/release/alitto/pond"/></a>
<a title="Go Report Card" target="_blank" href="https://goreportcard.com/report/github.com/alitto/pond/v2"><img src="https://goreportcard.com/badge/github.com/alitto/pond/v2"/></a>


<img src="./docs/logo.svg" height="100" />

pond is a minimalistic and high-performance Go library designed to elegantly manage concurrent tasks.

## Motivation

This library is meant to provide a simple and idiomatic way to manage concurrency in Go programs. Based on the [Worker Pool pattern](https://en.wikipedia.org/wiki/Thread_pool), it allows running a large number of tasks concurrently while limiting the number of goroutines that are running at the same time. This is useful when you need to limit the number of concurrent operations to avoid resource exhaustion or hitting rate limits.

Some common use cases include:
 - Processing a large number of tasks concurrently
 - Limiting the number of concurrent HTTP requests
 - Limiting the number of concurrent database connections
 - Sending HTTP requests to a rate-limited API

## Features:

- Zero dependencies
- Create pools of goroutines that scale automatically based on the number of tasks submitted
- Limit the number of concurrent tasks running at the same time
- Worker goroutines are only created when needed and immediately removed when idle (scale to zero)
- Minimalistic and fluent APIs for:
  - Creating worker pools with maximum number of workers
  - Submitting tasks to a pool and waiting for them to complete
  - Submitting tasks to a pool in a fire-and-forget fashion
  - Submitting a group of tasks and waiting for them to complete or the first error to occur
  - Stopping a worker pool
  - Monitoring pool metrics such as number of running workers, tasks waiting in the queue, etc.
- Very high performance and efficient resource usage under heavy workloads, even outperforming unbounded goroutines in some scenarios
- Complete pool metrics such as number of running workers, tasks waiting in the queue [and more](#metrics--monitoring)
- Configurable parent context to stop all workers when it is cancelled
- **New features in v2**:
  - Bounded or Unbounded task queues
  - Submission of tasks that return results
  - Awaitable task completion
  - Type safe APIs for tasks that return errors or results
  - Panics recovery (panics are captured and returned as errors)
  - Subpools with a fraction of the parent pool's maximum number of workers
  - Blocking and non-blocking submission of tasks when the queue is full
  - Dynamic resizing of the pool
- [API reference](https://pkg.go.dev/github.com/alitto/pond/v2)

## Installation

```bash
go get -u github.com/alitto/pond/v2
```

## Usage

### Submitting tasks to a pool with limited concurrency

``` go
package main

import (
	"fmt"

	"github.com/alitto/pond/v2"
)

func main() {

	// Create a pool with limited concurrency
	pool := pond.NewPool(100)

	// Submit 1000 tasks
	for i := 0; i < 1000; i++ {
		i := i
		pool.Submit(func() {
			fmt.Printf("Running task #%d\n", i)
		})
	}

	// Stop the pool and wait for all submitted tasks to complete
	pool.StopAndWait()
}
```

### Submitting tasks that return errors

This feature allows you to submit tasks that return an `error`. This is useful when you need to handle errors that occur during the execution of a task.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(100)

// Submit a task that returns an error
task := pool.SubmitErr(func() error {
	return errors.New("An error occurred")
})

// Wait for the task to complete and get the error
err := task.Wait()
```

### Submitting tasks that return results

This feature allows you to submit tasks that return a value. This is useful when you need to process the result of a task.

``` go
// Create a pool that accepts tasks that return a string and an error
pool := pond.NewResultPool[string](10)

// Submit a task that returns a string
task := pool.Submit(func() (string) {
	return "Hello, World!"
})

// Wait for the task to complete and get the result
result, err := task.Wait()
// result = "Hello, World!" and err = nil
```

### Submitting tasks that return results or errors

This feature allows you to submit tasks that return a value and an `error`. This is useful when you need to handle errors that occur during the execution of a task.

``` go
// Create a concurrency limited pool that accepts tasks that return a string
pool := pond.NewResultPool[string](10)

// Submit a task that returns a string value or an error
task := pool.SubmitErr(func() (string, error) {
	return "Hello, World!", nil
})

// Wait for the task to complete and get the result
result, err := task.Wait()
// result = "Hello, World!" and err = nil
```

### Submitting tasks associated with a context

If you need to submit a task that is associated with a context, you can pass the context directly to the task function.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a context that can be cancelled
ctx, cancel := context.WithCancel(context.Background())

// Submit a task that is associated with a context
task := pool.SubmitErr(func() error {
	return doSomethingWithCtx(ctx) // Pass the context to the task directly
})

// Wait for the task to complete and get the error.
// If the context is cancelled, the task is stopped and an error is returned.
err := task.Wait()
```

### Submitting a group of related tasks

You can submit a group of tasks that are related to each other. This is useful when you need to execute a group of tasks concurrently and wait for all of them to complete.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a task group
group := pool.NewGroup()

// Submit a group of tasks
for i := 0; i < 20; i++ {
	i := i
	group.Submit(func() {
		fmt.Printf("Running group task #%d\n", i)
	})
}

// Wait for all tasks in the group to complete
err := group.Wait()
```

### Submitting a group of related tasks associated with a context

You can submit a group of tasks that are linked to a context. This is useful when you need to execute a group of tasks concurrently and stop them when the context is cancelled (e.g. when the parent task is cancelled or times out).

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a context with a 5s timeout
timeout, _ := context.WithTimeout(context.Background(), 5*time.Second)

// Create a task group with a context
group := pool.NewGroupContext(timeout)

// Submit a group of tasks
for i := 0; i < 20; i++ {
	i := i
	group.Submit(func() {
		fmt.Printf("Running group task #%d\n", i)
	})
}

// Wait for all tasks in the group to complete or the timeout to occur, whichever comes first
err := group.Wait()
```

### Submitting a group of related tasks and waiting for the first error

You can submit a group of tasks that are related to each other and wait for the first error to occur. This is useful when you need to execute a group of tasks concurrently and stop the execution if an error occurs.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a task group
group := pool.NewGroup()

// Submit a group of tasks
for i := 0; i < 20; i++ {
	i := i
	group.SubmitErr(func() error {
		if n == 10 {
			return errors.New("An error occurred")
		}
		fmt.Printf("Running group task #%d\n", i)
		return nil
	})
}

// Wait for all tasks in the group to complete or the first error to occur
err := group.Wait()
```

### Submitting a group of related tasks that return results

You can submit a group of tasks that are related to each other and return results. This is useful when you need to execute a group of tasks concurrently and process the results. Results are returned in the order they were submitted.

``` go
// Create a pool with limited concurrency
pool := pond.NewResultPool[string](10)

// Create a task group
group := pool.NewGroup()

// Submit a group of tasks
for i := 0; i < 20; i++ {
	i := i
	group.Submit(func() string {
		return fmt.Sprintf("Task #%d", i)
	})
}

// Wait for all tasks in the group to complete
results, err := group.Wait()
// results = ["Task #0", "Task #1", ..., "Task #19"] and err = nil
```

### Stopping a group of tasks when a context is cancelled

If you need to submit a group of tasks that are associated with a context and stop them when the context is cancelled, you can pass the context directly to the task function.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a context that can be cancelled
ctx, cancel := context.WithCancel(context.Background())

// Create a task group
group := pool.NewGroupContext(ctx)

// Submit a group of tasks
for i := 0; i < 20; i++ {
	i := i
	group.SubmitErr(func() error {
		return doSomethingWithCtx(ctx) // Pass the context to the task directly
	})
}

// Wait for all tasks in the group to complete.
// If the context is cancelled, all tasks are stopped and the first error is returned.
err := group.Wait()
```

### Using a custom Context at the pool level

Each pool is associated with a context that is used to stop all workers when the pool is stopped. By default, the context is the background context (`context.Background()`). You can create a custom context and pass it to the pool to stop all workers when the context is cancelled.

```go
// Create a custom context that can be cancelled
customCtx, cancel := context.WithCancel(context.Background())

// This creates a pool that is stopped when customCtx is cancelled 
pool := pond.NewPool(10, pond.WithContext(customCtx))
``` 

### Stopping a pool

You can stop a pool using the `Stop` method. This will stop all workers and prevent new tasks from being submitted. You can also wait for all submitted tasks by calling the `Wait` method.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Submit a task
pool.Submit(func() {
	fmt.Println("Running task")
})

// Stop the pool and wait for all submitted tasks to complete
pool.Stop().Wait()
```

A shorthand method `StopAndWait` is also available to stop the pool and wait for all submitted tasks to complete.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Submit a task
pool.Submit(func() {
	fmt.Println("Running task")
})

// Stop the pool and wait for all submitted tasks to complete
pool.StopAndWait()
```

### Recovering from panics

By default, panics that occur during the execution of a task are captured and returned as errors. This allows you to recover from panics and handle them gracefully.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Submit a task that panics
task := pool.Submit(func() {
	panic("A panic occurred")
})

// Wait for the task to complete and get the error
err := task.Wait()

if err != nil {
	fmt.Printf("Failed to run task: %v", err)
} else {
	fmt.Println("Task completed successfully")
}
```

### Subpools (v2)

Subpools are pools that can have a fraction of the parent pool's maximum number of workers. This is useful when you need to create a pool of workers that can be used for a specific task or group of tasks.

``` go
// Create a pool with limited concurrency
pool := pond.NewPool(10)

// Create a subpool with a fraction of the parent pool's maximum number of workers
subpool := pool.NewSubpool(5)

// Submit a task to the subpool
subpool.Submit(func() {
	fmt.Println("Running task in subpool")
})

// Stop the subpool and wait for all submitted tasks to complete
subpool.StopAndWait()
```

### Default pool (v2)

The default pool is a global pool that is used when no pool is provided. This is useful when you need to submit tasks but don't want to create a pool explicitly.
The default pool does not have a maximum number of workers and scales automatically based on the number of tasks submitted.

``` go
// Submit a task to the default pool and wait for it to complete
err := pond.SubmitErr(func() error {
	fmt.Println("Running task in default pool")
	return nil
}).Wait()

if err != nil {
	fmt.Printf("Failed to run task: %v", err)
} else {
	fmt.Println("Task completed successfully")
}
```

### Bounded task queues (v2)

By default, task queues are unbounded, meaning that tasks are queued indefinitely until the pool is stopped (or the process runs out of memory). You can limit the number of tasks that can be queued by setting a queue size when creating a pool (`WithQueueSize` option).

``` go
// Create a pool with a maximum of 10 tasks in the queue
pool := pond.NewPool(1, pond.WithQueueSize(10))
```

**Blocking vs non-blocking task submission** 

When a pool defines a queue size (bounded), you can also specify how to handle tasks submitted when the queue is full. By default, task submission blocks until there is space in the queue (blocking mode), but you can change this behavior to non-blocking by setting the `WithNonBlocking` option to `true` when creating a pool. If the queue is full and non-blocking task submission is enabled, the task is dropped and an error is returned (`ErrQueueFull`).

``` go
// Create a pool with a maximum of 10 tasks in the queue and non-blocking task submission
pool := pond.NewPool(1, pond.WithQueueSize(10), pond.WithNonBlocking(true))
```

### Resizing pools (v2)

You can dynamically change the maximum number of workers in a pool using the `Resize` method. This is useful when you need to adjust the pool's capacity based on runtime conditions.

``` go
// Create a pool with 5 workers
pool := pond.NewPool(5)

// Submit some tasks
for i := 0; i < 20; i++ {
    pool.Submit(func() {
        // Do some work
    })
}

// Increase the pool size to 10 workers
pool.Resize(10)

// Submit more tasks that will use the increased capacity
for i := 0; i < 20; i++ {
    pool.Submit(func() {
        // Do some work
    })
}

// Decrease the pool size back to 5 workers
pool.Resize(5)
```

When resizing a pool:
- The new maximum concurrency must be greater than 0
- If you increase the size, new workers will be created as needed up to the new maximum
- If you decrease the size, existing workers will continue running until they complete their current tasks, but no new workers will be created until the number of running workers is below the new maximum

### Metrics & monitoring

Each worker pool instance exposes useful metrics that can be queried through the following methods:

- `pool.RunningWorkers() int64`: Current number of running workers
- `pool.SubmittedTasks() uint64`: Total number of tasks submitted since the pool was created
- `pool.WaitingTasks() uint64`: Current number of tasks in the queue that are waiting to be executed
- `pool.SuccessfulTasks() uint64`: Total number of tasks that have successfully completed their execution since the pool was created
- `pool.FailedTasks() uint64`: Total number of tasks that completed with panic since the pool was created
- `pool.CompletedTasks() uint64`: Total number of tasks that have completed their execution either successfully or with panic since the pool was created

In our [Prometheus example](./examples/prometheus/main.go) we showcase how to configure collectors for these metrics and expose them to Prometheus.

## Migrating from pond v1 to v2

If you are using pond v1, here are the changes you need to make to migrate to v2:

1. Update the import path to `github.com/alitto/pond/v2`
2. Replace `pond.New(100, 1000)` with `pond.NewPool(100)`. The second argument is no longer needed since task queues are unbounded by default.
3. The pool option `pond.Context` was renamed to `pond.WithContext`
4. The following pool options were deprecated:
   - `pond.MinWorkers`: This option is no longer needed since workers are created on demand and removed when idle.
   - `pond.IdleTimeout`: This option is no longer needed since workers are immediately removed when idle.
   - `pond.PanicHandler`: Panics are captured and returned as errors. You can handle panics by checking the error returned by the `Wait` method.
   - `pond.Strategy`: The pool now scales automatically based on the number of tasks submitted.
5. The `pool.StopAndWaitFor` method was deprecated. Use `pool.Stop().Done()` channel if you need to wait for the pool to stop in a select statement.
6. The `pool.Group` method was renamed to `pool.NewGroup`.
7. The `pool.GroupContext` was renamed to `pool.NewGroupWithContext`.


## Examples

You can find more examples in the [examples](./examples) directory.

## API Reference

Full API reference is available at https://pkg.go.dev/github.com/alitto/pond/v2

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
