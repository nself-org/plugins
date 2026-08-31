// Package handlers provides HTTP handlers for the audit-analytics plugin API.
package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/anomaly"
	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/patterns"
)

// QueryFn is an alias of anomaly.QueryFn so callers need not import sub-packages directly.
type QueryFn = anomaly.QueryFn

// ExecFn is an alias of anomaly.ExecFn.
type ExecFn = anomaly.ExecFn

// RefreshFn is a no-result statement executor (same shape as patterns.RefreshFn).
type RefreshFn = patterns.RefreshFn

// Handler holds injected DB functions for all analytics endpoints.
type Handler struct {
	query     QueryFn
	exec      ExecFn
	refresh   RefreshFn
	scorer    *anomaly.Scorer
	alerter   *anomaly.Alerter
	startedAt time.Time
}

// New creates a Handler.
func New(q QueryFn, e ExecFn, r RefreshFn, cfg anomaly.ScorerConfig, alertCfg anomaly.AlertConfig) *Handler {
	return &Handler{
		query:     q,
		exec:      e,
		refresh:   r,
		scorer:    anomaly.NewScorer(q, e, cfg),
		alerter:   anomaly.NewAlerter(alertCfg),
		startedAt: time.Now().UTC(),
	}
}

// RegisterRoutes mounts all analytics routes on mux, gated by the shared secret.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	secret := SharedSecret()
	mux.HandleFunc("GET /audit/analytics/anomalies", sharedSecretMiddleware(secret, h.listAnomalies))
	mux.HandleFunc("GET /audit/analytics/anomalies/{id}", sharedSecretMiddleware(secret, h.getAnomaly))
	mux.HandleFunc("PATCH /audit/analytics/anomalies/{id}", sharedSecretMiddleware(secret, h.reviewAnomaly))
	mux.HandleFunc("GET /audit/analytics/heatmap", sharedSecretMiddleware(secret, h.getHeatmap))
	mux.HandleFunc("GET /audit/analytics/top-actors", sharedSecretMiddleware(secret, h.topActors))
	mux.HandleFunc("POST /audit/analytics/refresh", sharedSecretMiddleware(secret, h.forceRefresh))
	mux.HandleFunc("GET /audit/analytics/status", h.status)
}

func (h *Handler) listAnomalies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	severity := q.Get("severity")
	from := q.Get("from")
	to := q.Get("to")
	tenantID := tenantIDFromRequest(r)

	// Tenant isolation (S40-fix, CR-C gate 2026-05-17):
	// In Cloud mode the tenant header is mandatory. An absent header means the
	// upstream caller is misconfigured — returning all rows would be a cross-tenant
	// data leak. Fail with 400 so the misconfiguration surfaces immediately.
	// In self-host mode (no header, no NSELF_DEPLOY_MODE=cloud) all rows are
	// returned — single-tenant deploy, no cross-tenant risk.
	if tenantID == "" && os.Getenv("NSELF_DEPLOY_MODE") == "cloud" {
		jsonError(w, "X-Audit-Tenant-Id header required in cloud mode", http.StatusBadRequest)
		return
	}

	sqlStr := `SELECT id, tenant_id, user_id, anomaly_type, severity, z_score,
	               detected_at, context, reviewed_by, reviewed_at, disposition
	          FROM np_audit_anomalies WHERE 1=1`
	args := []interface{}{}

	// Apply tenant filter when tenantID is present (Cloud mode or explicitly scoped self-host).
	if tenantID != "" {
		args = append(args, tenantID)
		sqlStr += " AND tenant_id = $" + strconv.Itoa(len(args))
	}

	if severity != "" {
		args = append(args, severity)
		sqlStr += " AND severity = $" + strconv.Itoa(len(args))
	}
	if from != "" {
		args = append(args, from)
		sqlStr += " AND detected_at >= $" + strconv.Itoa(len(args))
	}
	if to != "" {
		args = append(args, to)
		sqlStr += " AND detected_at <= $" + strconv.Itoa(len(args))
	}
	sqlStr += " ORDER BY detected_at DESC LIMIT 200"

	rows, err := h.query(r.Context(), sqlStr, args...)
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

func (h *Handler) getAnomaly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tenantID := tenantIDFromRequest(r)

	var (
		rows []map[string]interface{}
		err  error
	)
	if tenantID != "" {
		rows, err = h.query(r.Context(), `
			SELECT id, tenant_id, user_id, anomaly_type, severity, z_score,
			       detected_at, context, reviewed_by, reviewed_at, disposition
			  FROM np_audit_anomalies
			 WHERE id = $1 AND tenant_id = $2
		`, id, tenantID)
	} else {
		rows, err = h.query(r.Context(), `
			SELECT id, tenant_id, user_id, anomaly_type, severity, z_score,
			       detected_at, context, reviewed_by, reviewed_at, disposition
			  FROM np_audit_anomalies
			 WHERE id = $1
		`, id)
	}
	if err != nil || len(rows) == 0 {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, rows[0])
}

func (h *Handler) reviewAnomaly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		ReviewedBy  string `json:"reviewed_by"`
		Disposition string `json:"disposition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.ReviewedBy == "" || body.Disposition == "" {
		jsonError(w, "reviewed_by and disposition required", http.StatusBadRequest)
		return
	}

	tenantID := tenantIDFromRequest(r)
	var err error
	if tenantID != "" {
		_, err = h.exec(r.Context(), `
			UPDATE np_audit_anomalies
			   SET reviewed_by = $1, reviewed_at = NOW(), disposition = $2
			 WHERE id = $3 AND tenant_id = $4
		`, body.ReviewedBy, body.Disposition, id, tenantID)
	} else {
		_, err = h.exec(r.Context(), `
			UPDATE np_audit_anomalies
			   SET reviewed_by = $1, reviewed_at = NOW(), disposition = $2
			 WHERE id = $3
		`, body.ReviewedBy, body.Disposition, id)
	}
	if err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (h *Handler) getHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	tenantID := tenantIDFromRequest(r)
	rows, err := patterns.QueryHeatmap(r.Context(), h.query, userID, tenantID, days)
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

func (h *Handler) topActors(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	tenantID := tenantIDFromRequest(r)
	actors, err := patterns.TopActors(r.Context(), h.query, limit, days, tenantID)
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, actors)
}

func (h *Handler) forceRefresh(w http.ResponseWriter, r *http.Request) {
	if err := h.refresh(r.Context(), "REFRESH MATERIALIZED VIEW CONCURRENTLY np_audit_heatmap"); err != nil {
		jsonError(w, "refresh failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"ok": true, "refreshed_at": time.Now().UTC()})
}

func (h *Handler) status(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]interface{}{
		"started_at": h.startedAt,
		"uptime_sec": time.Since(h.startedAt).Seconds(),
	})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
