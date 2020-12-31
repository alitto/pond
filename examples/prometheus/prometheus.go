package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/alitto/pond"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {

	// Create a worker pool
	pool := pond.New(10, 0)

	// Register pool metrics collectors
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "pond_running_workers",
			Help: "Number of running worker goroutines",
		},
		func() float64 {
			return float64(pool.RunningWorkers())
		}))
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "pond_idle_workers",
			Help: "Number of idle worker goroutines",
		},
		func() float64 {
			return float64(pool.IdleWorkers())
		}))

	// Expose the registered metrics via HTTP
	http.Handle("/metrics", promhttp.Handler())

	go submitTasks(pool)

	// Start the server
	http.ListenAndServe(":8080", nil)

}

func submitTasks(pool *pond.WorkerPool) {

	// Submit 1000 tasks
	for i := 0; i < 1000; i++ {
		n := i
		pool.Submit(func() {
			fmt.Printf("Running task #%d\n", n)
			time.Sleep(3 * time.Second)
		})
	}
}
