package anomaly

import (
	"context"
	"fmt"
	"math"
)

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
				  AND user_id IS NOT NULL
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
			  AND user_id IS NOT NULL
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
		   AND user_id IS NOT NULL
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
			   AND user_id IS NOT NULL
			   AND context->>'ip' IS NOT NULL
		),
		recent AS (
			SELECT DISTINCT user_id, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '5 minutes'
			   AND user_id IS NOT NULL
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
			   AND user_id IS NOT NULL
			   AND action IN ('role_grant','schema_alter','plugin_install','gdpr_delete')
			   AND (EXTRACT(HOUR FROM created_at) >= $1 OR EXTRACT(HOUR FROM created_at) < $2)
		`, start, end)
	} else {
		rows, err = s.query(ctx, `
			SELECT user_id, action, context->>'ip' AS ip
			  FROM np_audit_log
			 WHERE created_at > NOW() - INTERVAL '5 minutes'
			   AND user_id IS NOT NULL
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
