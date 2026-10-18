package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"math"
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

// detectFrequencySpikes computes z-scores for (user, action, hour) against the 7-day baseline.
func (s *Scorer) detectFrequencySpikes(ctx context.Context) ([]Anomaly, error) {
	// Current window: last 5 minutes, grouped by (user_id, action, hour).
	// Baseline: events from NOW()-7d to NOW()-5m, same grouping, compute mean+stddev.
	rows, err := s.query(ctx, `
		WITH baseline AS (
			SELECT
				user_id,
				action,
				EXTRACT(HOUR FROM created_at) AS hour,
				AVG(cnt)    AS mean,
				STDDEV(cnt) AS stddev
			FROM (
				SELECT
					user_id,
					action,
					EXTRACT(HOUR FROM created_at) AS hour,
					DATE_TRUNC('hour', created_at) AS bucket,
					COUNT(*) AS cnt
				FROM np_audit_log
				WHERE created_at > NOW() - INTERVAL '7 days'
				  AND created_at < NOW() - INTERVAL '5 minutes'
				GROUP BY 1,2,3,4
			) buckets
			GROUP BY user_id, action, hour
		),
		current_window AS (
			SELECT
				user_id,
				action,
				EXTRACT(HOUR FROM created_at) AS hour,
				COUNT(*) AS cnt
			FROM np_audit_log
			WHERE created_at > NOW() - INTERVAL '5 minutes'
			GROUP BY user_id, action, EXTRACT(HOUR FROM created_at)
		)
		SELECT
			cw.user_id,
			cw.action,
			cw.cnt,
			b.mean,
			b.stddev,
			CASE WHEN b.stddev > 0 THEN (cw.cnt - b.mean) / b.stddev ELSE 0 END AS z_score
		FROM current_window cw
		JOIN baseline b ON b.user_id = cw.user_id AND b.action = cw.action AND b.hour = cw.hour
		WHERE b.stddev > 0
	`)
	if err != nil {
		return nil, fmt.Errorf("frequency spike query: %w", err)
	}

	var out []Anomaly
	for _, row := range rows {
		z, _ := toFloat(row["z_score"])
		if z < s.config.ZScoreThreshold {
			continue
		}
		cnt, _ := toFloat(row["cnt"])
		mean, _ := toFloat(row["mean"])
		out = append(out, Anomaly{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			AnomalyType: TypeFreqSpike,
			Severity:    severityForZScore(z),
			ZScore:      z,
			Context: map[string]interface{}{
				"action":   row["action"],
				"count":    cnt,
				"baseline": mean,
			},
		})
	}
	return out, nil
}

// detectBulkDeletes finds users who deleted 50+ rows in the last 1 minute.
func (s *Scorer) detectBulkDeletes(ctx context.Context) ([]Anomaly, error) {
	threshold := s.config.BulkDeleteThreshold
	if threshold <= 0 {
		threshold = 50
	}
	rows, err := s.query(ctx, `
		SELECT user_id, COUNT(*) AS cnt
		  FROM np_audit_log
		 WHERE action LIKE '%delete%'
		   AND created_at > NOW() - INTERVAL '1 minute'
		 GROUP BY user_id
		HAVING COUNT(*) >= $1
	`, threshold)
	if err != nil {
		return nil, fmt.Errorf("bulk delete query: %w", err)
	}

	var out []Anomaly
	for _, row := range rows {
		cnt, _ := toFloat(row["cnt"])
		z := math.Log10(cnt) + 1 // synthetic z for severity bucketing
		out = append(out, Anomaly{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			AnomalyType: TypeBulkDelete,
			Severity:    severityForZScore(z * 1.5), // amplify to reach HIGH faster
			ZScore:      z,
			Context: map[string]interface{}{
				"action": "delete",
				"count":  cnt,
			},
		})
	}
	return out, nil
}

// detectNewIPs finds users whose last 5 minutes include an IP not seen in the prior 7 days.
func (s *Scorer) detectNewIPs(ctx context.Context) ([]Anomaly, error) {
	rows, err := s.query(ctx, `
		WITH historical AS (
			SELECT DISTINCT user_id, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '7 days'
			   AND created_at < NOW() - INTERVAL '5 minutes'
			   AND context->>'ip' IS NOT NULL
		),
		recent AS (
			SELECT DISTINCT user_id, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '5 minutes'
			   AND context->>'ip' IS NOT NULL
		)
		SELECT r.user_id, r.ip
		  FROM recent r
		 WHERE NOT EXISTS (
			SELECT 1 FROM historical h
			 WHERE h.user_id = r.user_id AND h.ip = r.ip
		 )
	`)
	if err != nil {
		return nil, fmt.Errorf("new IP query: %w", err)
	}

	var out []Anomaly
	for _, row := range rows {
		out = append(out, Anomaly{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			AnomalyType: TypeNewIP,
			Severity:    SeverityMedium,
			ZScore:      3.5, // fixed above threshold
			Context: map[string]interface{}{
				"ip": row["ip"],
			},
		})
	}
	return out, nil
}

// detectAfterHoursAdmin finds admin actions outside business hours.
func (s *Scorer) detectAfterHoursAdmin(ctx context.Context) ([]Anomaly, error) {
	start := s.config.AfterHoursStart
	end := s.config.AfterHoursEnd

	// After-hours window wraps midnight (e.g. 20:00–06:00).
	var rows []map[string]interface{}
	var err error
	if start > end {
		rows, err = s.query(ctx, `
			SELECT user_id, action, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '5 minutes'
			   AND action IN ('role_grant','schema_alter','plugin_install','gdpr_delete')
			   AND (EXTRACT(HOUR FROM created_at) >= $1 OR EXTRACT(HOUR FROM created_at) < $2)
		`, start, end)
	} else {
		rows, err = s.query(ctx, `
			SELECT user_id, action, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '5 minutes'
			   AND action IN ('role_grant','schema_alter','plugin_install','gdpr_delete')
			   AND EXTRACT(HOUR FROM created_at) BETWEEN $1 AND $2
		`, start, end)
	}
	if err != nil {
		return nil, fmt.Errorf("after-hours query: %w", err)
	}

	var out []Anomaly
	for _, row := range rows {
		out = append(out, Anomaly{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			AnomalyType: TypeAfterHoursAdmin,
			Severity:    SeverityMedium,
			ZScore:      3.5,
			Context: map[string]interface{}{
				"action": row["action"],
				"ip":     row["ip"],
			},
		})
	}
	return out, nil
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
