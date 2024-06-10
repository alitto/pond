package pond

import (
	"context"
	"sync"
)

// worker represents a worker goroutine
func worker(context context.Context, waitGroup *sync.WaitGroup, firstTask func(), tasks <-chan func(), taskExecutor func(func(), bool), taskWaitGroup *sync.WaitGroup) {

	// If provided, execute the first task immediately, before listening to the tasks channel
	if firstTask != nil {
		taskExecutor(firstTask, true)
	}

	defer func() {
		waitGroup.Done()
	}()

	for {
		select {
		case <-context.Done():
			// Pool context was cancelled, empty tasks channel and exit
			for _ = range tasks {
				taskWaitGroup.Done()
			}
			return
		case task, ok := <-tasks:
			// Prioritize context.Done statement (https://stackoverflow.com/questions/46200343/force-priority-of-go-select-statement)
			select {
			case <-context.Done():
				if task != nil && ok {
					// We have received a task, ignore it
					taskWaitGroup.Done()
				}
			default:
				if task == nil || !ok {
					// We have received a signal to exit
					return
				}

				// We have received a task, execute it
				taskExecutor(task, false)
			}
		}
	}
}
