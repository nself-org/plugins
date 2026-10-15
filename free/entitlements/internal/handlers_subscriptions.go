// Purpose: entitlements HTTP handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: requests are scoped by source_account_id.
package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

// ============================================================
// Subscriptions
// ============================================================

func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		        price_cents, currency, trial_start, trial_end, current_period_start, current_period_end,
		        cancel_at_period_end, canceled_at, metadata, created_at, updated_at
		 FROM entitlement_subscriptions WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(
			&s.ID, &s.SourceAccountID, &s.WorkspaceID, &s.UserID, &s.PlanID, &s.Status,
			&s.BillingInterval, &s.PriceCents, &s.Currency, &s.TrialStart, &s.TrialEnd,
			&s.PeriodStart, &s.PeriodEnd, &s.CancelAtEnd, &s.CanceledAt, &s.Metadata,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []Subscription{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": subs})
}

func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	planID, _ := body["plan_id"].(string)
	if planID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id is required"})
		return
	}
	userID, _ := body["user_id"].(string)
	workspaceID, _ := body["workspace_id"].(string)
	if userID == "" && workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id or workspace_id is required"})
		return
	}
	billingInterval := "month"
	if v, ok := body["billing_interval"].(string); ok && v != "" {
		billingInterval = v
	}
	now := time.Now().UTC()
	var periodEnd time.Time
	if billingInterval == "year" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_subscriptions
		 (source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		  price_cents, currency, current_period_start, current_period_end)
		 VALUES ($1,$2,$3,$4,'active',$5,0,'USD',$6,$7)
		 RETURNING id`,
		sa, nullStr(workspaceID), nullStr(userID), planID, billingInterval, now, periodEnd,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var s Subscription
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
		        price_cents, currency, trial_start, trial_end, current_period_start, current_period_end,
		        cancel_at_period_end, canceled_at, metadata, created_at, updated_at
		 FROM entitlement_subscriptions WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&s.ID, &s.SourceAccountID, &s.WorkspaceID, &s.UserID, &s.PlanID, &s.Status,
		&s.BillingInterval, &s.PriceCents, &s.Currency, &s.TrialStart, &s.TrialEnd,
		&s.PeriodStart, &s.PeriodEnd, &s.CancelAtEnd, &s.CanceledAt, &s.Metadata,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions SET updated_at=NOW() WHERE id=$1 AND source_account_id=$2`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	now := time.Now().UTC()
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions SET status='canceled', canceled_at=$3, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("cancel: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Features
// ============================================================

func (h *Handlers) ListFeatures(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, key, name, description, feature_type, is_active, category,
		        metadata, created_at, updated_at
		 FROM entitlement_features WHERE source_account_id=$1 ORDER BY name`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		if err := rows.Scan(
			&f.ID, &f.SourceAccountID, &f.Key, &f.Name, &f.Description,
			&f.FeatureType, &f.IsActive, &f.Category, &f.Metadata, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		features = append(features, f)
	}
	if features == nil {
		features = []Feature{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": features})
}

func (h *Handlers) CreateFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	key, _ := body["key"].(string)
	name, _ := body["name"].(string)
	featureType, _ := body["feature_type"].(string)
	if key == "" || name == "" || featureType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key, name, and feature_type are required"})
		return
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_features (source_account_id, key, name, description, feature_type, is_active)
		 VALUES ($1,$2,$3,$4,$5,true) RETURNING id`,
		sa, key, name, body["description"], featureType,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) UpdateFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_features SET
		   name = COALESCE($3, name),
		   description = COALESCE($4, description),
		   updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableString(body, "name"),
		nullableString(body, "description"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeleteFeature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`DELETE FROM entitlement_features WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
