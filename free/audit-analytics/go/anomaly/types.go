// Package anomaly provides z-score based anomaly detection over np_audit_log events.
package anomaly

// Type constants for anomaly_type values stored in np_audit_anomalies.
const (
	TypeBulkDelete      = "bulk_delete"
	TypeNewIP           = "new_ip"
	TypeImpossibleTravel = "impossible_travel"
	TypeAfterHoursAdmin  = "after_hours_admin"
	TypeFreqSpike        = "freq_spike"
)

// Severity levels for np_audit_anomalies.severity.
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// PrivilegedActions is the set of actions that route to the review queue.
var PrivilegedActions = map[string]bool{
	"role_grant":    true,
	"plugin_install": true,
	"schema_alter":  true,
	"gdpr_delete":   true,
}

// Anomaly is an in-memory representation of a detected anomaly before DB persistence.
type Anomaly struct {
	TenantID    *string
	UserID      string
	AnomalyType string
	Severity    string
	ZScore      float64
	// Context holds extra detection metadata (action, count, baseline, ip, location).
	Context map[string]interface{}
}

// severityForZScore converts a z-score to a named severity bucket.
// Thresholds (conservative defaults; configurable via NSELF_AUDIT_ANOMALY_ZSCORE env):
//   3.0–4.9 → medium, 5.0–7.9 → high, 8.0+ → critical.
func severityForZScore(z float64) string {
	switch {
	case z >= 8.0:
		return SeverityCritical
	case z >= 5.0:
		return SeverityHigh
	case z >= 3.0:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
