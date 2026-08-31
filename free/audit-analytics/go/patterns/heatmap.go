// Package patterns provides materialized view refresh and pattern analysis queries.
package patterns

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/anomaly"
)

// RefreshFn executes a statement (no result needed).
type RefreshFn func(ctx context.Context, sql string, args ...interface{}) error

// QueryFn is an alias of anomaly.QueryFn — both packages share the same underlying type.
type QueryFn = anomaly.QueryFn

// ExecFn is an alias of anomaly.ExecFn — both packages share the same underlying type.
type ExecFn = anomaly.ExecFn

// HeatmapRefresher manages periodic refresh of np_audit_heatmap.
type HeatmapRefresher struct {
	exec     RefreshFn
	interval time.Duration
}

// NewHeatmapRefresher creates a HeatmapRefresher.
// interval is the time between REFRESH MATERIALIZED VIEW calls.
// Pass 0 to use the default of 1 hour.
func NewHeatmapRefresher(exec RefreshFn, interval time.Duration) *HeatmapRefresher {
	if interval <= 0 {
		interval = time.Hour
	}
	return &HeatmapRefresher{exec: exec, interval: interval}
}

// Run starts the refresh loop. Blocks until ctx is cancelled.
func (h *HeatmapRefresher) Run(ctx context.Context) {
	// Refresh immediately on start.
	h.Refresh(ctx)

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.Refresh(ctx)
		}
	}
}

// Refresh triggers REFRESH MATERIALIZED VIEW CONCURRENTLY for np_audit_heatmap.
// CONCURRENTLY avoids locking reads during refresh (requires a UNIQUE index).
func (h *HeatmapRefresher) Refresh(ctx context.Context) {
	if err := h.exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY np_audit_heatmap"); err != nil {
		slog.Warn("audit-analytics: heatmap refresh failed", "err", err)
	} else {
		slog.Info("audit-analytics: heatmap refreshed")
	}
}

// HeatmapRow is a single row from np_audit_heatmap.
type HeatmapRow struct {
	TenantID    *string `json:"tenant_id,omitempty"`
	UserID      string  `json:"user_id"`
	DayOfWeek   float64 `json:"day_of_week"` // 0=Sunday..6=Saturday
	HourOfDay   float64 `json:"hour_of_day"` // 0-23 UTC
	ActionCount int64   `json:"action_count"`
}

// QueryHeatmap returns heatmap rows for the given user over the last N days.
// Pass empty userID to return heatmap across all users (admin use).
// tenantID filters to a specific Cloud tenant; empty means self-host (no filter).
func QueryHeatmap(ctx context.Context, query QueryFn, userID, tenantID string, days int) ([]HeatmapRow, error) {
	if days <= 0 {
		days = 30
	}

	var rows []map[string]interface{}
	var err error

	switch {
	case userID == "" && tenantID == "":
		rows, err = query(ctx, `
			SELECT tenant_id, user_id, day_of_week, hour_of_day, action_count
			  FROM np_audit_heatmap
			 ORDER BY action_count DESC
		`)
	case userID == "" && tenantID != "":
		rows, err = query(ctx, `
			SELECT tenant_id, user_id, day_of_week, hour_of_day, action_count
			  FROM np_audit_heatmap
			 WHERE tenant_id = $1
			 ORDER BY action_count DESC
		`, tenantID)
	case userID != "" && tenantID == "":
		rows, err = query(ctx, `
			SELECT tenant_id, user_id, day_of_week, hour_of_day, action_count
			  FROM np_audit_heatmap
			 WHERE user_id = $1
			 ORDER BY day_of_week, hour_of_day
		`, userID)
	default: // both set
		rows, err = query(ctx, `
			SELECT tenant_id, user_id, day_of_week, hour_of_day, action_count
			  FROM np_audit_heatmap
			 WHERE user_id = $1 AND tenant_id = $2
			 ORDER BY day_of_week, hour_of_day
		`, userID, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("heatmap query: %w", err)
	}

	out := make([]HeatmapRow, 0, len(rows))
	for _, row := range rows {
		hr := HeatmapRow{
			UserID:      fmt.Sprintf("%v", row["user_id"]),
			DayOfWeek:   toFloat64(row["day_of_week"]),
			HourOfDay:   toFloat64(row["hour_of_day"]),
			ActionCount: toInt64(row["action_count"]),
		}
		if tid, ok := row["tenant_id"]; ok && tid != nil {
			s := fmt.Sprintf("%v", tid)
			hr.TenantID = &s
		}
		out = append(out, hr)
	}
	return out, nil
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	default:
		return 0
	}
}
