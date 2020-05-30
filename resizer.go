package pond

import (
	"container/ring"
	"math"
	"time"
)

// Preset pool resizing strategies
var (
	// Eager maximizes responsiveness at the expense of higher resource usage,
	// which can reduce throughput under certain conditions.
	// This strategy is meant for worker pools that will operate at a small percentage of their capacity
	// most of the time and may occasionally receive bursts of tasks.
	Eager = func() ResizingStrategy { return DynamicResizer(1, 0.01) }
	// Balanced tries to find a balance between responsiveness and throughput.
	// It's the default strategy and it's suitable for general purpose worker pools or those
	// that will operate close to 50% of their capacity most of the time.
	Balanced = func() ResizingStrategy { return DynamicResizer(3, 0.01) }
	// Lazy maximizes throughput at the expense of responsiveness.
	// This strategy is meant for worker pools that will operate close to their max. capacity most of the time.
	Lazy = func() ResizingStrategy { return DynamicResizer(5, 0.01) }
)

// dynamicResizer implements a configurable dynamic resizing strategy
type dynamicResizer struct {
	windowSize     int
	tolerance      float64
	incomingTasks  *ring.Ring
	completedTasks *ring.Ring
	duration       *ring.Ring
	busyWorkers    *ring.Ring
}

// DynamicResizer creates a dynamic resizing strategy that gradually increases or decreases
// the size of the pool to match the rate of incoming tasks (input rate) with the rate of
// completed tasks (output rate).
// windowSize: determines how many cycles to consider when calculating input and output rates.
// tolerance: defines a percentage (between 0 and 1)
func DynamicResizer(windowSize int, tolerance float64) ResizingStrategy {

	if windowSize < 1 {
		windowSize = 1
	}
	if tolerance < 0 {
		tolerance = 0
	}

	dynamicResizer := &dynamicResizer{
		windowSize: windowSize,
		tolerance:  tolerance,
	}
	dynamicResizer.reset()
	return dynamicResizer
}

func (r *dynamicResizer) reset() {
	// Create rings
	r.incomingTasks = ring.New(r.windowSize)
	r.completedTasks = ring.New(r.windowSize)
	r.duration = ring.New(r.windowSize)
	r.busyWorkers = ring.New(r.windowSize)

	// Initialize with 0s
	for i := 0; i < r.windowSize; i++ {
		r.incomingTasks.Value = 0
		r.completedTasks.Value = 0
		r.duration.Value = 0 * time.Second
		r.busyWorkers.Value = 0
		r.incomingTasks = r.incomingTasks.Next()
		r.completedTasks = r.completedTasks.Next()
		r.duration = r.duration.Next()
		r.busyWorkers = r.busyWorkers.Next()
	}
}

func (r *dynamicResizer) totalIncomingTasks() int {
	var valueSum int = 0
	r.incomingTasks.Do(func(value interface{}) {
		valueSum += value.(int)
	})
	return valueSum
}

func (r *dynamicResizer) totalCompletedTasks() int {
	var valueSum int = 0
	r.completedTasks.Do(func(value interface{}) {
		valueSum += value.(int)
	})
	return valueSum
}

func (r *dynamicResizer) totalDuration() time.Duration {
	var valueSum time.Duration = 0
	r.duration.Do(func(value interface{}) {
		valueSum += value.(time.Duration)
	})
	return valueSum
}

func (r *dynamicResizer) avgBusyWorkers() float64 {
	var valueSum int = 0
	r.busyWorkers.Do(func(value interface{}) {
		valueSum += value.(int)
	})
	return float64(valueSum) / float64(r.windowSize)
}

func (r *dynamicResizer) push(incomingTasks, completedTasks, busyWorkers int, duration time.Duration) {
	r.incomingTasks.Value = incomingTasks
	r.completedTasks.Value = completedTasks
	r.duration.Value = duration
	r.busyWorkers.Value = busyWorkers
	r.incomingTasks = r.incomingTasks.Next()
	r.completedTasks = r.completedTasks.Next()
	r.duration = r.duration.Next()
	r.busyWorkers = r.busyWorkers.Next()
}

func (r *dynamicResizer) Resize(runningWorkers, idleWorkers, minWorkers, maxWorkers, incomingTasks, completedTasks int, duration time.Duration) int {

	r.push(incomingTasks, completedTasks, runningWorkers-idleWorkers, duration)

	windowIncomingTasks := r.totalIncomingTasks()
	windowCompletedTasks := r.totalCompletedTasks()
	windowSecs := r.totalDuration().Seconds()
	windowInputRate := float64(windowIncomingTasks) / windowSecs
	windowOutputRate := float64(windowCompletedTasks) / windowSecs

	if runningWorkers == 0 || windowCompletedTasks == 0 {
		// No workers yet, create as many workers ar.incomingTasks-idleWorkers
		delta := incomingTasks - idleWorkers
		if delta < 0 {
			delta = 0
		}
		return r.fitDelta(delta, runningWorkers, minWorkers, maxWorkers)
	}

	// Calculate max throughput
	avgBusyWorkers := r.avgBusyWorkers()
	if avgBusyWorkers < 1 {
		avgBusyWorkers = 1
	}
	windowWorkerRate := windowOutputRate / avgBusyWorkers
	if windowWorkerRate < 1 {
		windowWorkerRate = 1
	}
	maxOutputRate := windowWorkerRate * float64(runningWorkers)

	deltaRate := windowInputRate - maxOutputRate

	// No changes, do not resize
	if deltaRate == 0 {
		return 0
	}

	// If delta % is below the defined tolerance, do not resize
	if r.tolerance > 0 {
		deltaPercentage := math.Abs(deltaRate / windowInputRate)
		if deltaPercentage < r.tolerance {
			return 0
		}
	}

	if deltaRate > 0 {
		ratio := windowSecs / float64(r.windowSize)
		delta := int(ratio * (deltaRate / windowWorkerRate))
		if delta < 0 {
			delta = 0
		}
		if deltaRate > 0 && delta < 1 {
			delta = 1
		}
		return r.fitDelta(delta, runningWorkers, minWorkers, maxWorkers)
	} else if deltaRate < 0 && idleWorkers > 0 {
		// Need to shrink the pool
		return r.fitDelta(-1, runningWorkers, minWorkers, maxWorkers)
	}
	return 0
}

func (r *dynamicResizer) fitDelta(delta, current, min, max int) int {
	if current+delta < min {
		delta = -(current - min)
	}
	if current+delta > max {
		delta = max - current
	}
	return delta
}
