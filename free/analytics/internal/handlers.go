package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// sa returns the source_account_id from the X-Source-Account-ID header, defaulting to "primary".
func sa(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		return "primary"
	}
	// normalize: lowercase, replace non-alphanumeric (except _ -) with -, trim dashes
	v = strings.ToLower(v)
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	v = re.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "primary"
	}
	return v
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a JSON error response.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ─── Health ───────────────────────────────────────────────────────────────────

// HandleHealth returns 200 ok.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "analytics",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReady checks database connectivity.
func HandleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ready":     false,
				"plugin":    "analytics",
				"error":     "Database unavailable",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ready":     true,
			"plugin":    "analytics",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ─── Events ───────────────────────────────────────────────────────────────────

// HandleTrackEvent handles POST /api/v1/events.
func HandleTrackEvent(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TrackEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.EventName == "" {
			errJSON(w, http.StatusBadRequest, "event_name is required")
			return
		}

		sourceAccountID := sa(r)
		id, err := insertEvent(r.Context(), db, sourceAccountID, req)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to track event")
			return
		}

		// auto-increment counter
		dim := "total"
		if req.UserID != "" {
			dim = req.UserID
		}
		_ = incrementCounter(r.Context(), db, sourceAccountID, req.EventName, dim, 1)

		writeJSON(w, http.StatusOK, map[string]any{"success": true, "event_id": id})
	}
}

// HandleTrackEventBatch handles POST /api/v1/events/batch.
func HandleTrackEventBatch(db *DB, batchMax int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TrackEventBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if len(req.Events) == 0 {
			errJSON(w, http.StatusBadRequest, "events array cannot be empty")
			return
		}
		if batchMax > 0 && len(req.Events) > batchMax {
			errJSON(w, http.StatusBadRequest, fmt.Sprintf("batch size exceeds maximum of %d", batchMax))
			return
		}

		sourceAccountID := sa(r)
		count := 0
		for _, ev := range req.Events {
			if _, err := insertEvent(r.Context(), db, sourceAccountID, ev); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to track batch")
				return
			}
			count++
			dim := "total"
			if ev.UserID != "" {
				dim = ev.UserID
			}
			_ = incrementCounter(r.Context(), db, sourceAccountID, ev.EventName, dim, 1)
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true, "tracked": count})
	}
}

// HandleListEvents handles GET /api/v1/events.
func HandleListEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := queryInt(q.Get("limit"), 100)
		offset := queryInt(q.Get("offset"), 0)

		sourceAccountID := sa(r)

		sql := `SELECT id, source_account_id, event_name, event_category, user_id, session_id,
			properties, context, source_plugin, timestamp, created_at
			FROM analytics_events WHERE source_account_id = $1`
		args := []any{sourceAccountID}
		idx := 2

		if v := q.Get("name"); v != "" {
			sql += fmt.Sprintf(" AND event_name = $%d", idx)
			args = append(args, v)
			idx++
		}
		if v := q.Get("user_id"); v != "" {
			sql += fmt.Sprintf(" AND user_id = $%d", idx)
			args = append(args, v)
			idx++
		}
		if v := q.Get("start_date"); v != "" {
			sql += fmt.Sprintf(" AND timestamp >= $%d", idx)
			args = append(args, v)
			idx++
		}
		if v := q.Get("end_date"); v != "" {
			sql += fmt.Sprintf(" AND timestamp <= $%d", idx)
			args = append(args, v)
			idx++
		}

		countSQL := strings.Replace(sql, `SELECT id, source_account_id, event_name, event_category, user_id, session_id,
			properties, context, source_plugin, timestamp, created_at`, "SELECT COUNT(*)", 1)
		var total int64
		_ = db.Pool.QueryRow(r.Context(), countSQL, args...).Scan(&total)

		sql += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d OFFSET $%d", idx, idx+1)
		args = append(args, limit, offset)

		rows, err := db.Pool.Query(r.Context(), sql, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to query events")
			return
		}
		defer rows.Close()

		events := make([]Event, 0)
		for rows.Next() {
			var e Event
			var props, ctx []byte
			if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.EventName, &e.EventCategory,
				&e.UserID, &e.SessionID, &props, &ctx, &e.SourcePlugin, &e.Timestamp, &e.CreatedAt); err != nil {
				continue
			}
			_ = json.Unmarshal(props, &e.Properties)
			_ = json.Unmarshal(ctx, &e.Context)
			events = append(events, e)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":   events,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// ─── Counters ─────────────────────────────────────────────────────────────────

// HandleIncrementCounter handles POST /api/v1/counters/increment.
func HandleIncrementCounter(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req IncrementCounterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.CounterName == "" {
			errJSON(w, http.StatusBadRequest, "counter_name is required")
			return
		}
		dim := req.Dimension
		if dim == "" {
			dim = "total"
		}
		amount := req.Amount
		if amount == 0 {
			amount = 1
		}

		if err := incrementCounter(r.Context(), db, sa(r), req.CounterName, dim, amount); err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to increment counter")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	}
}

// HandleListCounters handles GET /api/v1/counters.
func HandleListCounters(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := queryInt(q.Get("limit"), 100)
		offset := queryInt(q.Get("offset"), 0)

		rows, err := db.Pool.Query(r.Context(),
			`SELECT DISTINCT ON (counter_name, dimension, period)
				counter_name, dimension, period, period_start, value, updated_at
			 FROM analytics_counters
			 WHERE source_account_id = $1
			 ORDER BY counter_name, dimension, period, period_start DESC
			 LIMIT $2 OFFSET $3`,
			sa(r), limit, offset)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list counters")
			return
		}
		defer rows.Close()

		counters := make([]Counter, 0)
		for rows.Next() {
			var c Counter
			if err := rows.Scan(&c.CounterName, &c.Dimension, &c.Period, &c.PeriodStart, &c.Value, &c.UpdatedAt); err != nil {
				continue
			}
			counters = append(counters, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": counters, "limit": limit, "offset": offset})
	}
}

// HandleGetCounter handles GET /api/v1/counters/{key}.
func HandleGetCounter(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		q := r.URL.Query()
		dim := q.Get("dimension")
		if dim == "" {
			dim = "total"
		}
		period := q.Get("period")
		if period == "" {
			period = "all_time"
		}

		var c Counter
		err := db.Pool.QueryRow(r.Context(),
			`SELECT counter_name, dimension, period, period_start, value, updated_at
			 FROM analytics_counters
			 WHERE source_account_id = $1 AND counter_name = $2 AND dimension = $3 AND period = $4
			 ORDER BY period_start DESC LIMIT 1`,
			sa(r), key, dim, period).
			Scan(&c.CounterName, &c.Dimension, &c.Period, &c.PeriodStart, &c.Value, &c.UpdatedAt)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "counter not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to get counter")
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

// ─── Funnels ──────────────────────────────────────────────────────────────────

// HandleCreateFunnel handles POST /api/v1/funnels.
func HandleCreateFunnel(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateFunnelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Name == "" || len(req.Steps) == 0 {
			errJSON(w, http.StatusBadRequest, "name and steps are required")
			return
		}
		wh := req.WindowHours
		if wh == 0 {
			wh = 24
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		stepsJSON, _ := json.Marshal(req.Steps)
		var id string
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO analytics_funnels
			 (source_account_id, name, description, steps, window_hours, enabled, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
			 RETURNING id`,
			sa(r), req.Name, nilStr(req.Description), stepsJSON, wh, enabled).Scan(&id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to create funnel")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "funnel_id": id})
	}
}

// HandleListFunnels handles GET /api/v1/funnels.
func HandleListFunnels(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := queryInt(q.Get("limit"), 100)
		offset := queryInt(q.Get("offset"), 0)

		rows, err := db.Pool.Query(r.Context(),
			`SELECT id, source_account_id, name, description, steps, window_hours, enabled, created_at, updated_at
			 FROM analytics_funnels WHERE source_account_id = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			sa(r), limit, offset)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list funnels")
			return
		}
		defer rows.Close()

		funnels := scanFunnels(rows)
		writeJSON(w, http.StatusOK, map[string]any{"data": funnels, "limit": limit, "offset": offset})
	}
}

// HandleAnalyzeFunnel handles GET /api/v1/funnels/{id}/analyze.
func HandleAnalyzeFunnel(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sourceAccountID := sa(r)

		funnel, err := getFunnel(r.Context(), db, sourceAccountID, id)
		if err == pgx.ErrNoRows || funnel == nil {
			errJSON(w, http.StatusNotFound, "funnel not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to fetch funnel")
			return
		}

		type rawStep struct {
			StepNumber int
			StepName   string
			Users      int64
		}
		rawSteps := make([]rawStep, 0, len(funnel.Steps))

		windowMS := int64(funnel.WindowHours) * 3600000

		for i, step := range funnel.Steps {
			var count int64
			if i == 0 {
				_ = db.Pool.QueryRow(r.Context(),
					`SELECT COUNT(DISTINCT user_id) FROM analytics_events
					 WHERE source_account_id = $1 AND event_name = $2 AND user_id IS NOT NULL`,
					sourceAccountID, step.EventName).Scan(&count)
			} else {
				prev := funnel.Steps[i-1]
				_ = db.Pool.QueryRow(r.Context(),
					fmt.Sprintf(`SELECT COUNT(DISTINCT e1.user_id)
					 FROM analytics_events e1
					 INNER JOIN analytics_events e2
					   ON e1.user_id = e2.user_id
					   AND e2.event_name = $3
					   AND e2.timestamp >= e1.timestamp
					   AND e2.timestamp <= e1.timestamp + INTERVAL '%d milliseconds'
					 WHERE e1.source_account_id = $1 AND e1.event_name = $2 AND e1.user_id IS NOT NULL`,
						windowMS),
					sourceAccountID, prev.EventName, step.EventName).Scan(&count)
			}
			rawSteps = append(rawSteps, rawStep{i + 1, step.Name, count})
		}

		var totalEntered, totalCompleted int64
		if len(rawSteps) > 0 {
			totalEntered = rawSteps[0].Users
			totalCompleted = rawSteps[len(rawSteps)-1].Users
		}

		steps := make([]FunnelStepResult, 0, len(rawSteps))
		for idx, s := range rawSteps {
			var prevUsers int64
			if idx == 0 {
				prevUsers = totalEntered
			} else {
				prevUsers = rawSteps[idx-1].Users
			}
			cr := float64(0)
			dor := float64(0)
			if prevUsers > 0 {
				cr = round2(float64(s.Users) / float64(prevUsers) * 100)
				dor = round2(float64(prevUsers-s.Users) / float64(prevUsers) * 100)
			}
			eventName := ""
			if idx < len(funnel.Steps) {
				eventName = funnel.Steps[idx].EventName
			}
			steps = append(steps, FunnelStepResult{
				StepNumber:     s.StepNumber,
				StepName:       s.StepName,
				EventName:      eventName,
				Users:          s.Users,
				ConversionRate: cr,
				DropOffRate:    dor,
			})
		}

		ocr := float64(0)
		if totalEntered > 0 {
			ocr = round2(float64(totalCompleted) / float64(totalEntered) * 100)
		}

		writeJSON(w, http.StatusOK, FunnelAnalysis{
			FunnelID:              id,
			FunnelName:            funnel.Name,
			Steps:                 steps,
			TotalEntered:          totalEntered,
			TotalCompleted:        totalCompleted,
			OverallConversionRate: ocr,
			AnalysisTimestamp:     time.Now().UTC(),
		})
	}
}

// ─── Quotas ───────────────────────────────────────────────────────────────────

// HandleCreateQuota handles POST /api/v1/quotas.
func HandleCreateQuota(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateQuotaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Name == "" || req.CounterName == "" || req.MaxValue == 0 || req.Period == "" {
			errJSON(w, http.StatusBadRequest, "name, counter_name, max_value, and period are required")
			return
		}
		scope := req.Scope
		if scope == "" {
			scope = "app"
		}
		action := req.ActionOnExceed
		if action == "" {
			action = "warn"
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		var id string
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO analytics_quotas
			 (source_account_id, name, scope, scope_id, counter_name, max_value, period, action_on_exceed, enabled, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
			 RETURNING id`,
			sa(r), req.Name, scope, nilStr(req.ScopeID), req.CounterName, req.MaxValue, req.Period, action, enabled).Scan(&id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to create quota")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "quota_id": id})
	}
}

// HandleListQuotas handles GET /api/v1/quotas.
func HandleListQuotas(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := queryInt(q.Get("limit"), 100)
		offset := queryInt(q.Get("offset"), 0)

		rows, err := db.Pool.Query(r.Context(),
			`SELECT id, source_account_id, name, scope, scope_id, counter_name, max_value, period,
			        action_on_exceed, enabled, created_at, updated_at
			 FROM analytics_quotas WHERE source_account_id = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			sa(r), limit, offset)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list quotas")
			return
		}
		defer rows.Close()

		quotas := make([]Quota, 0)
		for rows.Next() {
			var qr Quota
			if err := rows.Scan(&qr.ID, &qr.SourceAccountID, &qr.Name, &qr.Scope, &qr.ScopeID,
				&qr.CounterName, &qr.MaxValue, &qr.Period, &qr.ActionOnExceed, &qr.Enabled,
				&qr.CreatedAt, &qr.UpdatedAt); err != nil {
				continue
			}
			quotas = append(quotas, qr)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": quotas, "limit": limit, "offset": offset})
	}
}

// HandleListViolations handles GET /api/v1/violations.
func HandleListViolations(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := queryInt(q.Get("limit"), 100)
		offset := queryInt(q.Get("offset"), 0)

		rows, err := db.Pool.Query(r.Context(),
			`SELECT id, source_account_id, quota_id, scope_id, current_value, max_value,
			        action_taken, notified, created_at
			 FROM analytics_quota_violations WHERE source_account_id = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			sa(r), limit, offset)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list violations")
			return
		}
		defer rows.Close()

		violations := make([]QuotaViolation, 0)
		for rows.Next() {
			var v QuotaViolation
			if err := rows.Scan(&v.ID, &v.SourceAccountID, &v.QuotaID, &v.ScopeID,
				&v.CurrentValue, &v.MaxValue, &v.ActionTaken, &v.Notified, &v.CreatedAt); err != nil {
				continue
			}
			violations = append(violations, v)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": violations, "limit": limit, "offset": offset})
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// HandleStats handles GET /api/v1/stats.
func HandleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := sa(r)
		ctx := r.Context()

		var totalEvents, uniqueUsers int64
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM analytics_events WHERE source_account_id = $1`, sourceAccountID).Scan(&totalEvents)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(DISTINCT user_id) FROM analytics_events WHERE source_account_id = $1 AND user_id IS NOT NULL`,
			sourceAccountID).Scan(&uniqueUsers)

		topRows, err := db.Pool.Query(ctx,
			`SELECT event_name, COUNT(*) as cnt FROM analytics_events
			 WHERE source_account_id = $1
			 GROUP BY event_name ORDER BY cnt DESC LIMIT 10`, sourceAccountID)
		topEvents := make([]TopEvent, 0)
		if err == nil {
			defer topRows.Close()
			for topRows.Next() {
				var te TopEvent
				if err := topRows.Scan(&te.EventName, &te.Count); err == nil {
					topEvents = append(topEvents, te)
				}
			}
		}

		writeJSON(w, http.StatusOK, Stats{
			Events:      totalEvents,
			UniqueUsers: uniqueUsers,
			TopEvents:   topEvents,
		})
	}
}

// HandleStatsTimeseries handles GET /api/v1/stats/timeseries.
func HandleStatsTimeseries(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		eventName := q.Get("event_name")
		interval := q.Get("interval")
		if interval == "" {
			interval = "day"
		}
		startDate := q.Get("start_date")
		endDate := q.Get("end_date")
		sourceAccountID := sa(r)
		ctx := r.Context()

		sql := `SELECT DATE_TRUNC($1, timestamp) as ts, COUNT(*) as cnt
			FROM analytics_events WHERE source_account_id = $2`
		args := []any{interval, sourceAccountID}
		idx := 3

		if eventName != "" {
			sql += fmt.Sprintf(" AND event_name = $%d", idx)
			args = append(args, eventName)
			idx++
		}
		if startDate != "" {
			sql += fmt.Sprintf(" AND timestamp >= $%d", idx)
			args = append(args, startDate)
			idx++
		}
		if endDate != "" {
			sql += fmt.Sprintf(" AND timestamp <= $%d", idx)
			args = append(args, endDate)
			idx++
		}
		sql += " GROUP BY ts ORDER BY ts ASC"

		rows, err := db.Pool.Query(ctx, sql, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to query timeseries")
			return
		}
		defer rows.Close()

		points := make([]TimeseriesPoint, 0)
		for rows.Next() {
			var p TimeseriesPoint
			if err := rows.Scan(&p.Timestamp, &p.Count); err == nil {
				points = append(points, p)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": points})
	}
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func insertEvent(ctx context.Context, db *DB, sourceAccountID string, req TrackEventRequest) (string, error) {
	props, _ := json.Marshal(req.Properties)
	ctxJSON, _ := json.Marshal(req.Context)

	var ts time.Time
	if req.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			ts = t
		} else {
			ts = time.Now().UTC()
		}
	} else {
		ts = time.Now().UTC()
	}

	var id string
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO analytics_events
		 (source_account_id, event_name, event_category, user_id, session_id, properties, context, source_plugin, timestamp, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		 RETURNING id`,
		sourceAccountID,
		req.EventName,
		nilStr(req.EventCategory),
		nilStr(req.UserID),
		nilStr(req.SessionID),
		props,
		ctxJSON,
		nilStr(req.SourcePlugin),
		ts,
	).Scan(&id)
	return id, err
}

func incrementCounter(ctx context.Context, db *DB, sourceAccountID, counterName, dimension string, amount int64) error {
	now := time.Now().UTC()
	type periodDef struct {
		period string
		start  time.Time
	}
	periods := []periodDef{
		{"hourly", time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)},
		{"daily", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)},
		{"monthly", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)},
		{"all_time", time.Unix(0, 0).UTC()},
	}
	for _, p := range periods {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO analytics_counters
			 (source_account_id, counter_name, dimension, period, period_start, value, metadata, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, '{}', NOW())
			 ON CONFLICT (source_account_id, counter_name, dimension, period, period_start)
			 DO UPDATE SET value = analytics_counters.value + EXCLUDED.value, updated_at = NOW()`,
			sourceAccountID, counterName, dimension, p.period, p.start, amount)
		if err != nil {
			return err
		}
	}
	return nil
}

func getFunnel(ctx context.Context, db *DB, sourceAccountID, id string) (*Funnel, error) {
	var f Funnel
	var stepsRaw []byte
	err := db.Pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, description, steps, window_hours, enabled, created_at, updated_at
		 FROM analytics_funnels WHERE id = $1 AND source_account_id = $2`,
		id, sourceAccountID).
		Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &stepsRaw, &f.WindowHours, &f.Enabled, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(stepsRaw, &f.Steps)
	return &f, nil
}

func scanFunnels(rows pgx.Rows) []Funnel {
	funnels := make([]Funnel, 0)
	for rows.Next() {
		var f Funnel
		var stepsRaw []byte
		if err := rows.Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &stepsRaw,
			&f.WindowHours, &f.Enabled, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		_ = json.Unmarshal(stepsRaw, &f.Steps)
		funnels = append(funnels, f)
	}
	return funnels
}

// nilStr returns nil if s is empty, otherwise a pointer to s.
func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// queryInt parses s as int, returning def on failure.
func queryInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
