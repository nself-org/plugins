// Purpose: entitlements HTTP handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: requests are scoped by source_account_id.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"time"
)

// Handlers holds the database pool and handles all HTTP requests.
type Handlers struct {
	pool PgxIface
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(pool PgxIface) *Handlers {
	return &Handlers{pool: pool}
}

// sa returns the source account ID from the request header, defaulting to "primary".
func (h *Handlers) sa(r *http.Request) string {
	if sa := r.Header.Get("X-Source-Account-ID"); sa != "" {
		return sa
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("response encoding: %v", err)
	}
}

// ============================================================
// Health / Status
// ============================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "plugin": "entitlements", "migrations": bootMigrationsHealth(w)})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"ready": "false", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ready": "true", "plugin": "entitlements"})
}

func (h *Handlers) PluginStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plugin": "entitlements", "version": "1.0.0", "status": "running"})
}

// ============================================================
// Plans
// ============================================================

func (h *Handlers) ListPlans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, name, slug, description, billing_interval, price_cents, currency,
		        trial_days, plan_type, is_public, is_archived, features, quotas, metadata, display_order,
		        created_at, updated_at
		 FROM entitlement_plans WHERE source_account_id=$1 AND is_archived=false ORDER BY display_order, created_at`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.Name, &p.Slug, &p.Description,
			&p.BillingInterval, &p.PriceCents, &p.Currency, &p.TrialDays,
			&p.PlanType, &p.IsPublic, &p.IsArchived, &p.Features, &p.Quotas,
			&p.Metadata, &p.DisplayOrder, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		plans = append(plans, p)
	}
	if plans == nil {
		plans = []Plan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": plans})
}

func (h *Handlers) CreatePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	name, _ := body["name"].(string)
	slug, _ := body["slug"].(string)
	billingInterval, _ := body["billing_interval"].(string)
	planType, _ := body["plan_type"].(string)
	if name == "" || slug == "" || billingInterval == "" || planType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, slug, billing_interval, and plan_type are required"})
		return
	}
	priceCents := 0
	if v, ok := body["price_cents"].(float64); ok {
		priceCents = int(v)
	}
	currency := "USD"
	if v, ok := body["currency"].(string); ok && v != "" {
		currency = v
	}
	trialDays := 0
	if v, ok := body["trial_days"].(float64); ok {
		trialDays = int(v)
	}
	isPublic := true
	if v, ok := body["is_public"].(bool); ok {
		isPublic = v
	}
	features := body["features"]
	if features == nil {
		features = map[string]any{}
	}
	quotas := body["quotas"]
	if quotas == nil {
		quotas = map[string]any{}
	}
	displayOrder := 0
	if v, ok := body["display_order"].(float64); ok {
		displayOrder = int(v)
	}
	featuresJSON, _ := json.Marshal(features)
	quotasJSON, _ := json.Marshal(quotas)
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_plans
		 (source_account_id, name, slug, description, billing_interval, price_cents, currency,
		  trial_days, plan_type, is_public, is_archived, features, quotas, display_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false,$11,$12,$13)
		 RETURNING id`,
		sa, name, slug, body["description"], billingInterval, priceCents, currency,
		trialDays, planType, isPublic, featuresJSON, quotasJSON, displayOrder,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var p Plan
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, slug, description, billing_interval, price_cents, currency,
		        trial_days, plan_type, is_public, is_archived, features, quotas, metadata, display_order,
		        created_at, updated_at
		 FROM entitlement_plans WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&p.ID, &p.SourceAccountID, &p.Name, &p.Slug, &p.Description,
		&p.BillingInterval, &p.PriceCents, &p.Currency, &p.TrialDays,
		&p.PlanType, &p.IsPublic, &p.IsArchived, &p.Features, &p.Quotas,
		&p.Metadata, &p.DisplayOrder, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_plans SET
		   name = COALESCE($3, name),
		   description = COALESCE($4, description),
		   price_cents = COALESCE($5, price_cents),
		   trial_days = COALESCE($6, trial_days),
		   is_public = COALESCE($7, is_public),
		   updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableString(body, "name"),
		nullableString(body, "description"),
		nullableInt(body, "price_cents"),
		nullableInt(body, "trial_days"),
		nullableBool(body, "is_public"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeletePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_plans SET is_archived=true, updated_at=NOW() WHERE id=$1 AND source_account_id=$2`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("archive: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Helpers
// ============================================================

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableString(body map[string]any, key string) any {
	v, ok := body[key].(string)
	if !ok || v == "" {
		return nil
	}
	return v
}

func nullableInt(body map[string]any, key string) any {
	v, ok := body[key].(float64)
	if !ok {
		return nil
	}
	return int(v)
}

func nullableBool(body map[string]any, key string) any {
	v, ok := body[key].(bool)
	if !ok {
		return nil
	}
	return v
}
