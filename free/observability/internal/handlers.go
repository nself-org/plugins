package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/plugins/free/shared-utils/httpclient"
)

// watchdogClient is the package-level HTTP client used for watchdog liveness
// probes against tracked services. 30s budget per spec; per-call ctx timeouts
// are the actual operating budget.
var watchdogClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// Handlers holds the pool and watchdog state.
type Handlers struct {
	pool   *pgxpool.Pool
	cfg    Config
	mu     sync.Mutex
	wdStop chan struct{}
	wdLast *time.Time
	wdStart *time.Time
}

// NewHandlers creates a Handlers instance.
func NewHandlers(pool *pgxpool.Pool, cfg Config) *Handlers {
	return &Handlers{pool: pool, cfg: cfg}
}

// sa returns the source account id from the request header.
func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// =========================================================================
// Lifecycle
// =========================================================================

// StartWatchdog starts the background watchdog if configured.
func (h *Handlers) StartWatchdog() {
	if !h.cfg.WatchdogEnabled {
		return
	}
	h.mu.Lock()
	if h.wdStop != nil {
		h.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	h.wdStop = stop
	now := time.Now()
	h.wdStart = &now
	h.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Duration(h.cfg.CheckIntervalSeconds) * time.Second)
		defer ticker.Stop()
		// run once immediately
		h.runWatchdogCycle(context.Background())
		for {
			select {
			case <-ticker.C:
				h.runWatchdogCycle(context.Background())
			case <-stop:
				return
			}
		}
	}()
}

// StopWatchdog halts the background watchdog.
func (h *Handlers) StopWatchdog() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.wdStop != nil {
		close(h.wdStop)
		h.wdStop = nil
		h.wdStart = nil
	}
}

func (h *Handlers) runWatchdogCycle(ctx context.Context) {
	now := time.Now()
	h.mu.Lock()
	h.wdLast = &now
	h.mu.Unlock()

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at
         FROM np_observability_services ORDER BY id`)
	if err != nil {
		return
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.Name, &s.URL, &s.CheckType,
			&s.CheckIntervalSec, &s.TimeoutSec, &s.Status, &s.LastCheckedAt,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		services = append(services, s)
	}
	rows.Close()

	for _, svc := range services {
		h.checkService(ctx, svc)
	}

	// Cleanup old history
	retainDays := h.cfg.HealthHistoryRetainDays
	h.pool.Exec(ctx, //nolint:errcheck
		fmt.Sprintf(`DELETE FROM np_observability_health_history WHERE checked_at < NOW() - INTERVAL '%d days'`, retainDays))
}

func (h *Handlers) checkService(ctx context.Context, svc Service) {
	timeout := time.Duration(svc.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = time.Duration(h.cfg.WatchdogTimeoutSeconds) * time.Second
	}

	start := time.Now()
	var status, errMsg string
	var statusCode *int
	var rtMs int

	if svc.URL != nil && *svc.URL != "" {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, *svc.URL, nil)
		if err != nil {
			status = "error"
			errMsg = err.Error()
		} else {
			resp, err := watchdogClient.Do(req)
			rtMs = int(time.Since(start).Milliseconds())
			if err != nil {
				if reqCtx.Err() != nil {
					status = "timeout"
					errMsg = fmt.Sprintf("timed out after %ds", svc.TimeoutSec)
				} else {
					status = "error"
					errMsg = err.Error()
				}
			} else {
				resp.Body.Close()
				code := resp.StatusCode
				statusCode = &code
				if resp.StatusCode < 400 {
					status = "up"
				} else if resp.StatusCode >= 500 {
					status = "down"
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				} else {
					status = "degraded"
					errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
	} else {
		status = "unknown"
		errMsg = "no URL configured"
	}

	rtMsPtr := &rtMs
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}

	prevStatus := svc.Status

	// Record history
	h.pool.Exec(ctx, //nolint:errcheck
		`INSERT INTO np_observability_health_history
         (source_account_id, service_id, status, response_time_ms, status_code, error_message)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		svc.SourceAccountID, svc.ID, status, rtMsPtr, statusCode, errPtr)

	// Update service status
	h.pool.Exec(ctx, //nolint:errcheck
		`UPDATE np_observability_services SET status=$1, last_checked_at=NOW(), updated_at=NOW() WHERE id=$2`,
		status, svc.ID)

	// Emit watchdog event on status change
	if prevStatus != status {
		eventType := "status_changed"
		if status == "down" {
			eventType = "service_down"
		} else if prevStatus == "down" && status == "up" {
			eventType = "service_recovered"
		}
		details := fmt.Sprintf("Service %q changed from %s to %s", svc.Name, prevStatus, status)
		h.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO np_observability_watchdog_events (source_account_id, service_id, event_type, details)
             VALUES ($1, $2, $3, $4)`,
			svc.SourceAccountID, svc.ID, eventType, details)
	}
}

// =========================================================================
// Health / Ready
// =========================================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"plugin":    "observability",
		"timestamp": time.Now().UTC(),
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// =========================================================================
// Services
// =========================================================================

func (h *Handlers) ListServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at
         FROM np_observability_services WHERE source_account_id=$1 ORDER BY name ASC LIMIT $2`,
		sa, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.Name, &s.URL, &s.CheckType,
			&s.CheckIntervalSec, &s.TimeoutSec, &s.Status, &s.LastCheckedAt,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		services = append(services, s)
	}
	if services == nil {
		services = []Service{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": services, "count": len(services)})
}

func (h *Handlers) CreateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		Name             string  `json:"name"`
		URL              *string `json:"url"`
		CheckType        string  `json:"check_type"`
		CheckIntervalSec *int    `json:"check_interval_sec"`
		TimeoutSec       *int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	checkType := body.CheckType
	if checkType == "" {
		checkType = "http"
	}
	checkInterval := 30
	if body.CheckIntervalSec != nil {
		checkInterval = *body.CheckIntervalSec
	}
	timeoutSec := 10
	if body.TimeoutSec != nil {
		timeoutSec = *body.TimeoutSec
	}

	var s Service
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_observability_services
         (source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status)
         VALUES ($1, $2, $3, $4, $5, $6, 'unknown')
         RETURNING id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at`,
		sa, body.Name, body.URL, checkType, checkInterval, timeoutSec).
		Scan(&s.ID, &s.SourceAccountID, &s.Name, &s.URL, &s.CheckType,
			&s.CheckIntervalSec, &s.TimeoutSec, &s.Status, &s.LastCheckedAt,
			&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}

	// Emit discovery event
	h.pool.Exec(ctx, //nolint:errcheck
		`INSERT INTO np_observability_watchdog_events (source_account_id, service_id, event_type, details)
         VALUES ($1, $2, 'service_registered', $3)`,
		sa, s.ID, fmt.Sprintf("Service %q registered", s.Name))

	writeJSON(w, http.StatusCreated, s)
}

func (h *Handlers) GetService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var s Service
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at
         FROM np_observability_services WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&s.ID, &s.SourceAccountID, &s.Name, &s.URL, &s.CheckType,
			&s.CheckIntervalSec, &s.TimeoutSec, &s.Status, &s.LastCheckedAt,
			&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) UpdateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var body struct {
		Name             *string `json:"name"`
		URL              *string `json:"url"`
		CheckType        *string `json:"check_type"`
		CheckIntervalSec *int    `json:"check_interval_sec"`
		TimeoutSec       *int    `json:"timeout_sec"`
		Status           *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	var s Service
	err := h.pool.QueryRow(ctx,
		`UPDATE np_observability_services SET
             name             = COALESCE($3, name),
             url              = COALESCE($4, url),
             check_type       = COALESCE($5, check_type),
             check_interval_sec = COALESCE($6, check_interval_sec),
             timeout_sec      = COALESCE($7, timeout_sec),
             status           = COALESCE($8, status),
             updated_at       = NOW()
         WHERE id=$1 AND source_account_id=$2
         RETURNING id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at`,
		id, sa, body.Name, body.URL, body.CheckType, body.CheckIntervalSec, body.TimeoutSec, body.Status).
		Scan(&s.ID, &s.SourceAccountID, &s.Name, &s.URL, &s.CheckType,
			&s.CheckIntervalSec, &s.TimeoutSec, &s.Status, &s.LastCheckedAt,
			&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) DeleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_observability_services WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =========================================================================
// Manual health check trigger
// =========================================================================

func (h *Handlers) CheckService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var svc Service
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, url, check_type, check_interval_sec, timeout_sec, status, last_checked_at, created_at, updated_at
         FROM np_observability_services WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&svc.ID, &svc.SourceAccountID, &svc.Name, &svc.URL, &svc.CheckType,
			&svc.CheckIntervalSec, &svc.TimeoutSec, &svc.Status, &svc.LastCheckedAt,
			&svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}

	h.checkService(ctx, svc)

	// Return fresh row
	h.GetService(w, r)
}

// =========================================================================
// Health History
// =========================================================================

func (h *Handlers) ListServiceHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	serviceID := chi.URLParam(r, "id")

	q := r.URL.Query()
	limit := 100
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	conds := "WHERE source_account_id=$1 AND service_id=$2"
	args := []any{sa, serviceID}
	idx := 3

	if v := q.Get("start_date"); v != "" {
		conds += fmt.Sprintf(" AND checked_at >= $%d", idx)
		args = append(args, v)
		idx++
	}
	if v := q.Get("end_date"); v != "" {
		conds += fmt.Sprintf(" AND checked_at <= $%d", idx)
		args = append(args, v)
		idx++
	}
	_ = idx

	rows, err := h.pool.Query(ctx,
		fmt.Sprintf(`SELECT id, source_account_id, service_id, status, response_time_ms, status_code, error_message, checked_at
         FROM np_observability_health_history %s ORDER BY checked_at DESC LIMIT %d`, conds, limit),
		args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var history []HealthHistory
	for rows.Next() {
		var hh HealthHistory
		if err := rows.Scan(&hh.ID, &hh.SourceAccountID, &hh.ServiceID, &hh.Status,
			&hh.ResponseTimeMs, &hh.StatusCode, &hh.ErrorMessage, &hh.CheckedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		history = append(history, hh)
	}
	if history == nil {
		history = []HealthHistory{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history, "count": len(history)})
}

// =========================================================================
// Overall Health Summary
// =========================================================================

func (h *Handlers) HealthSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	row := h.pool.QueryRow(ctx,
		`SELECT
             COUNT(*) FILTER (WHERE TRUE) AS total,
             COUNT(*) FILTER (WHERE status='up') AS up,
             COUNT(*) FILTER (WHERE status='down') AS down,
             COUNT(*) FILTER (WHERE status='degraded') AS degraded,
             COUNT(*) FILTER (WHERE status='unknown') AS unknown
         FROM np_observability_services WHERE source_account_id=$1`, sa)

	var summary HealthSummary
	summary.Timestamp = time.Now().UTC()
	if err := row.Scan(&summary.Total, &summary.Up, &summary.Down, &summary.Degraded, &summary.Unknown); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// =========================================================================
// Watchdog Events
// =========================================================================

func (h *Handlers) ListWatchdogEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	q := r.URL.Query()
	limit := 100
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, service_id, event_type, details, occurred_at
         FROM np_observability_watchdog_events WHERE source_account_id=$1
         ORDER BY occurred_at DESC LIMIT $2`, sa, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var events []WatchdogEvent
	for rows.Next() {
		var e WatchdogEvent
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.ServiceID, &e.EventType, &e.Details, &e.OccurredAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		events = append(events, e)
	}
	if events == nil {
		events = []WatchdogEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}

func (h *Handlers) CreateWatchdogEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		ServiceID *string `json:"service_id"`
		EventType string  `json:"event_type"`
		Details   *string `json:"details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.EventType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_type is required"})
		return
	}

	var e WatchdogEvent
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_observability_watchdog_events (source_account_id, service_id, event_type, details)
         VALUES ($1, $2, $3, $4)
         RETURNING id, source_account_id, service_id, event_type, details, occurred_at`,
		sa, body.ServiceID, body.EventType, body.Details).
		Scan(&e.ID, &e.SourceAccountID, &e.ServiceID, &e.EventType, &e.Details, &e.OccurredAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// =========================================================================
// Stats
// =========================================================================

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var stats Stats
	row := h.pool.QueryRow(ctx,
		`SELECT
             COUNT(*) AS total,
             COUNT(*) FILTER (WHERE status='up') AS up,
             COUNT(*) FILTER (WHERE status='down') AS down,
             COUNT(*) FILTER (WHERE status='degraded') AS degraded
         FROM np_observability_services WHERE source_account_id=$1`, sa)
	if err := row.Scan(&stats.Total, &stats.Up, &stats.Down, &stats.Degraded); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query services: %v", err)})
		return
	}

	// avg response time from recent history (last 24h)
	var avg *float64
	avgRow := h.pool.QueryRow(ctx,
		`SELECT AVG(response_time_ms)
         FROM np_observability_health_history
         WHERE source_account_id=$1 AND checked_at > NOW() - INTERVAL '24 hours'
           AND response_time_ms IS NOT NULL`, sa)
	if err := avgRow.Scan(&avg); err == nil && avg != nil {
		n := int(*avg)
		stats.AvgResponseMs = &n
	}

	writeJSON(w, http.StatusOK, stats)
}

// =========================================================================
// Incidents
// =========================================================================

func (h *Handlers) ListIncidents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, service_id, event_type, details, occurred_at
         FROM np_observability_watchdog_events
         WHERE source_account_id=$1 AND event_type IN ('service_down','health_check_failed','status_changed')
         ORDER BY occurred_at DESC LIMIT 200`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var incidents []WatchdogEvent
	for rows.Next() {
		var e WatchdogEvent
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.ServiceID, &e.EventType, &e.Details, &e.OccurredAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		incidents = append(incidents, e)
	}
	if incidents == nil {
		incidents = []WatchdogEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents, "count": len(incidents)})
}
