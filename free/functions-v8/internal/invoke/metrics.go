// Package invoke provides Prometheus metrics for edge function invocations.
package invoke

import (
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	invocationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "nself",
			Subsystem: "edge_functions",
			Name:      "invocations_total",
			Help:      "Total number of edge function invocations.",
		},
		[]string{"function", "status_code"},
	)

	invocationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "nself",
			Subsystem: "edge_functions",
			Name:      "invocation_duration_seconds",
			Help:      "Duration of edge function invocations in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 5.0, 30.0},
		},
		[]string{"function"},
	)

	invocationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "nself",
			Subsystem: "edge_functions",
			Name:      "errors_total",
			Help:      "Total number of edge function invocation errors.",
		},
		[]string{"function", "error_type"},
	)
)

// poolAvailableGauge tracks available pool slots.
var poolAvailableGauge = promauto.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "nself",
		Subsystem: "edge_functions",
		Name:      "pool_slots_available",
		Help:      "Number of isolate pool slots currently available.",
	},
)

// poolTotalGauge tracks total pool capacity.
var poolTotalGauge = promauto.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "nself",
		Subsystem: "edge_functions",
		Name:      "pool_slots_total",
		Help:      "Total isolate pool capacity.",
	},
)

// lastInvocationTimestamp records the most recent invocation unix time (for Grafana).
var lastInvocationTimestamp uint64

// RecordInvocation records metrics for a completed invocation.
func RecordInvocation(functionName string, statusCode int, duration time.Duration, timedOut bool) {
	sc := strconv.Itoa(statusCode)
	invocationTotal.WithLabelValues(functionName, sc).Inc()
	invocationDuration.WithLabelValues(functionName).Observe(duration.Seconds())
	if timedOut {
		invocationErrors.WithLabelValues(functionName, "timeout").Inc()
	} else if statusCode >= 500 {
		invocationErrors.WithLabelValues(functionName, "server_error").Inc()
	}
	// Store unix time as float64 bits.
	atomic.StoreUint64(&lastInvocationTimestamp, math.Float64bits(float64(time.Now().Unix())))
}

// RecordPoolState updates the pool-slots gauges.
// available is the number of free slots; total is the pool capacity.
func RecordPoolState(available, total int) {
	poolAvailableGauge.Set(float64(available))
	poolTotalGauge.Set(float64(total))
}

// Handler returns an HTTP handler for the /metrics Prometheus endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
