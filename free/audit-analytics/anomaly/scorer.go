package anomaly

import (
	"context"
	"log/slog"
	"time"
)

// QueryFn is a generic DB query function compatible with pgxpool.Pool.
type QueryFn func(ctx context.Context, sql string, args ...interface{}) ([]map[string]interface{}, error)

// ExecFn executes a statement and returns rows affected.
type ExecFn func(ctx context.Context, sql string, args ...interface{}) (int64, error)

// ScorerConfig holds tunable parameters for the anomaly scorer.
type ScorerConfig struct {
	// ZScoreThreshold is the minimum z-score that produces an anomaly record.
	// Default: 3.0 (matches NSELF_AUDIT_ANOMALY_ZSCORE env var).
	ZScoreThreshold float64

	// BulkDeleteThreshold is the minimum number of delete events in 1 minute
	// to trigger a bulk_delete anomaly regardless of z-score.
	BulkDeleteThreshold int

	// AfterHoursStart and AfterHoursEnd define the UTC hour window for
	// after_hours_admin detection (0-23 inclusive). Default: 20–06.
	AfterHoursStart int
	AfterHoursEnd   int

	// PrivilegedReviewTTL is how long until a privileged review is overdue.
	PrivilegedReviewTTL time.Duration
}

// DefaultScorerConfig returns safe default configuration.
func DefaultScorerConfig() ScorerConfig {
	return ScorerConfig{
		ZScoreThreshold:     3.0,
		BulkDeleteThreshold: 50,
		AfterHoursStart:     20,
		AfterHoursEnd:       6,
		PrivilegedReviewTTL: 24 * time.Hour,
	}
}

// Scorer runs anomaly detection over recent audit_log events.
type Scorer struct {
	query  QueryFn
	exec   ExecFn
	config ScorerConfig
}

// NewScorer creates a Scorer.
func NewScorer(q QueryFn, e ExecFn, cfg ScorerConfig) *Scorer {
	return &Scorer{query: q, exec: e, config: cfg}
}

// ScoreRecent scores audit events from the last 5 minutes and writes anomaly records.
// Intended to be called on a cron schedule (every 5 minutes via the cron plugin).
func (s *Scorer) ScoreRecent(ctx context.Context) (int, error) {
	scored := 0

	// 1. Frequency spike detection (z-score over rolling 7-day baseline).
	spikes, err := s.detectFrequencySpikes(ctx)
	if err != nil {
		slog.Warn("anomaly: frequency spike detection failed", "err", err)
	}
	for _, a := range spikes {
		if err := s.writeAnomaly(ctx, a); err != nil {
			slog.Warn("anomaly: write failed", "type", a.AnomalyType, "err", err)
			continue
		}
		scored++
	}

	// 2. Bulk delete detection.
	bulkDeletes, err := s.detectBulkDeletes(ctx)
	if err != nil {
		slog.Warn("anomaly: bulk delete detection failed", "err", err)
	}
	for _, a := range bulkDeletes {
		if err := s.writeAnomaly(ctx, a); err != nil {
			slog.Warn("anomaly: write failed", "type", a.AnomalyType, "err", err)
			continue
		}
		scored++
	}

	// 3. New IP detection (user seen from an IP not in their 7-day history).
	newIPs, err := s.detectNewIPs(ctx)
	if err != nil {
		slog.Warn("anomaly: new IP detection failed", "err", err)
	}
	for _, a := range newIPs {
		if err := s.writeAnomaly(ctx, a); err != nil {
			slog.Warn("anomaly: write failed", "type", a.AnomalyType, "err", err)
			continue
		}
		scored++
	}

	// 4. After-hours admin actions.
	afterHours, err := s.detectAfterHoursAdmin(ctx)
	if err != nil {
		slog.Warn("anomaly: after-hours detection failed", "err", err)
	}
	for _, a := range afterHours {
		if err := s.writeAnomaly(ctx, a); err != nil {
			slog.Warn("anomaly: write failed", "type", a.AnomalyType, "err", err)
			continue
		}
		scored++
	}

	// 5. Queue privileged actions for review.
	if err := s.queuePrivilegedReviews(ctx); err != nil {
		slog.Warn("anomaly: privileged review queue failed", "err", err)
	}

	return scored, nil
}

// queuePrivilegedReviews inserts review records for recent privileged actions.
// Idempotent: ON CONFLICT DO NOTHING.
func (s *Scorer) queuePrivilegedReviews(ctx context.Context) error {
	ttlSeconds := int64(s.config.PrivilegedReviewTTL.Seconds())
	_, err := s.exec(ctx, `
		INSERT INTO np_audit_privileged_reviews (audit_log_id, action, actor_id, due_at)
		SELECT id, action, user_id, created_at + ($1 || ' seconds')::INTERVAL
		  FROM np_audit_log
		 WHERE created_at > NOW() - INTERVAL '5 minutes'
		   AND action IN ('role_grant', 'plugin_install', 'schema_alter', 'gdpr_delete')
		ON CONFLICT DO NOTHING
	`, ttlSeconds)
	return err
}

// writeAnomaly persists an Anomaly to np_audit_anomalies.
func (s *Scorer) writeAnomaly(ctx context.Context, a Anomaly) error {
	_, err := s.exec(ctx, `
		INSERT INTO np_audit_anomalies
			(tenant_id, user_id, anomaly_type, severity, z_score, context)
		VALUES
			($1, $2, $3, $4, $5, $6)
	`,
		a.TenantID,
		a.UserID,
		a.AnomalyType,
		a.Severity,
		a.ZScore,
		a.Context,
	)
	return err
}

// toFloat safely converts an interface value to float64.
func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	default:
		return 0, false
	}
}
