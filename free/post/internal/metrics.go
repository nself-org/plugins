package internal

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	postRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "post_requests_total",
			Help: "Total HTTP requests by endpoint, method, and status.",
		},
		[]string{"endpoint", "method", "status"},
	)
	postRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "post_request_duration_seconds",
			Help:    "HTTP request duration in seconds by endpoint.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)
	postErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "post_errors_total",
			Help: "Total errors by error type.",
		},
		[]string{"error_type"},
	)
	postMetricsRegistry = prometheus.NewRegistry()
)

func init() {
	postMetricsRegistry.MustRegister(postRequestsTotal, postRequestDuration, postErrorsTotal)
}

// PostMetricsHandler returns the Prometheus metrics HTTP handler.
func PostMetricsHandler() http.Handler {
	return promhttp.HandlerFor(postMetricsRegistry, promhttp.HandlerOpts{})
}

// PostMetricsMiddleware records request count and duration for each route.
func PostMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &postStatusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		endpoint := chi.RouteContext(r.Context()).RoutePattern()
		if endpoint == "" {
			endpoint = r.URL.Path
		}
		postRequestsTotal.WithLabelValues(endpoint, r.Method, strconv.Itoa(ww.status)).Inc()
		postRequestDuration.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
		if ww.status >= 500 {
			postErrorsTotal.WithLabelValues("server_error").Inc()
		} else if ww.status >= 400 {
			postErrorsTotal.WithLabelValues("client_error").Inc()
		}
	})
}

type postStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *postStatusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
