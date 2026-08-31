// Package metrics registers Prometheus metrics for the payments plugin.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus counters, histograms, and gauges for the payments plugin.
type Metrics struct {
	WebhookDuration      *prometheus.HistogramVec
	WebhookTotal         *prometheus.CounterVec
	CheckoutDuration     *prometheus.HistogramVec
	SubscriptionStatus   *prometheus.CounterVec
	DunningTransitions   *prometheus.CounterVec
	DunningRetry         *prometheus.CounterVec
	UsageRecords         *prometheus.CounterVec
	QueueDepth           prometheus.Gauge
}

// New creates and registers all metrics.
func New() *Metrics {
	return &Metrics{
		WebhookDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "nself_payments_webhook_duration_seconds",
			Help:    "End-to-end webhook handler latency",
			Buckets: []float64{0.01, 0.04, 0.12, 0.25, 0.5, 1.0},
		}, []string{"provider", "event_type"}),

		WebhookTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "nself_payments_webhook_total",
			Help: "Webhook events received",
		}, []string{"provider", "event_type", "status"}),

		CheckoutDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "nself_payments_checkout_duration_seconds",
			Help:    "Checkout session creation latency",
			Buckets: []float64{0.05, 0.1, 0.3, 0.6, 1.0, 2.0},
		}, []string{"provider"}),

		SubscriptionStatus: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "nself_payments_subscription_status_total",
			Help: "Subscription status transitions",
		}, []string{"provider", "status"}),

		DunningTransitions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "nself_payments_dunning_transitions_total",
			Help: "Dunning state machine transitions",
		}, []string{"from_status", "to_status"}),

		DunningRetry: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "nself_payments_dunning_retry_total",
			Help: "Retry attempts per dunning cycle",
		}, []string{"provider", "attempt"}),

		UsageRecords: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "nself_payments_usage_records_total",
			Help: "Usage units recorded",
		}, []string{"metric_id"}),

		QueueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "nself_payments_queue_depth",
			Help: "Unprocessed rows in np_payment_events",
		}),
	}
}
