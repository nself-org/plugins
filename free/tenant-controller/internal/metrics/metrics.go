// Package metrics exposes Prometheus metrics for the tenant-controller.
// All metrics defined in the B46 spec are registered here.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ProjectsTotal is a gauge of current projects by status (active/suspended/deleting).
	ProjectsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nself_controller_projects_total",
		Help: "Number of projects by status.",
	}, []string{"status"})

	// ProvisionDuration tracks provisioning latency per step.
	ProvisionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nself_controller_provision_duration_seconds",
		Help:    "Project provisioning latency per step.",
		Buckets: prometheus.DefBuckets,
	}, []string{"step"})

	// ProvisionTotal counts project create outcomes.
	ProvisionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nself_controller_provision_total",
		Help: "Total project create attempts by status (success/failure).",
	}, []string{"status"})

	// TeardownTotal counts project delete outcomes.
	TeardownTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nself_controller_teardown_total",
		Help: "Total project delete attempts by status (success/failure).",
	}, []string{"status"})

	// ProjectMemoryBytes tracks estimated memory per active project.
	ProjectMemoryBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nself_controller_project_memory_bytes",
		Help: "Estimated memory usage per active project.",
	}, []string{"slug"})

	// ProjectConnections tracks active DB connections per project schema.
	ProjectConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nself_controller_project_connections",
		Help: "Active database connections per project schema.",
	}, []string{"slug"})

	// TenantCount is an alias for total active projects.
	TenantCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nself_controller_tenant_count",
		Help: "Total active tenants (alias for projects_total{status=active}).",
	})

	// NginxReloadDuration tracks Nginx reload latency.
	NginxReloadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "nself_controller_nginx_reload_duration_seconds",
		Help:    "Nginx configuration reload latency.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
	})
)

// RecordProvisionStep records a provision step latency observation.
func RecordProvisionStep(step string, seconds float64) {
	ProvisionDuration.WithLabelValues(step).Observe(seconds)
}

// RecordProvisionOutcome increments the provision counter for the given status.
func RecordProvisionOutcome(success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	ProvisionTotal.WithLabelValues(status).Inc()
}

// RecordTeardownOutcome increments the teardown counter for the given status.
func RecordTeardownOutcome(success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	TeardownTotal.WithLabelValues(status).Inc()
}

// UpdateProjectCounts refreshes the projects_total gauge and tenant_count alias.
func UpdateProjectCounts(active, suspended, deleting int) {
	ProjectsTotal.WithLabelValues("active").Set(float64(active))
	ProjectsTotal.WithLabelValues("suspended").Set(float64(suspended))
	ProjectsTotal.WithLabelValues("deleting").Set(float64(deleting))
	TenantCount.Set(float64(active))
}
