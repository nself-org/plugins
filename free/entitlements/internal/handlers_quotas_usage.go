// Purpose: entitlements HTTP handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: requests are scoped by source_account_id.
package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
)

// ============================================================
// Quotas
// ============================================================

func (h *Handlers) ListQuotas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, plan_id, feature_id, limit_value, period, created_at
		 FROM entitlement_quotas WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var quotas []Quota
	for rows.Next() {
		var q Quota
		if err := rows.Scan(&q.ID, &q.SourceAccountID, &q.PlanID, &q.FeatureID, &q.LimitValue, &q.Period, &q.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		quotas = append(quotas, q)
	}
	if quotas == nil {
		quotas = []Quota{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": quotas})
}

func (h *Handlers) CreateQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	planID, _ := body["plan_id"].(string)
	featureID, _ := body["feature_id"].(string)
	if planID == "" || featureID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id and feature_id are required"})
		return
	}
	period := "monthly"
	if v, ok := body["period"].(string); ok && v != "" {
		period = v
	}
	var limitValue *int
	if v, ok := body["limit_value"].(float64); ok {
		iv := int(v)
		limitValue = &iv
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_quotas (source_account_id, plan_id, feature_id, limit_value, period)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		sa, planID, featureID, limitValue, period,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) UpdateQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_quotas SET
		   limit_value = COALESCE($3, limit_value),
		   period = COALESCE($4, period)
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		nullableInt(body, "limit_value"),
		nullableString(body, "period"),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handlers) DeleteQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`DELETE FROM entitlement_quotas WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ============================================================
// Usage
// ============================================================

func (h *Handlers) TrackUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID, _ := body["user_id"].(string)
	featureKey, _ := body["feature_key"].(string)
	if featureKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key is required"})
		return
	}
	amount := 1
	if v, ok := body["amount"].(float64); ok {
		amount = int(v)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_usage (source_account_id, user_id, quota_key, usage_amount)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		sa, nullStr(userID), featureKey, amount,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

func (h *Handlers) GetUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()
	userID := q.Get("user_id")
	featureKey := q.Get("feature_key")
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, quota_key, usage_amount, created_at
		 FROM entitlement_usage
		 WHERE source_account_id=$1
		   AND ($2='' OR user_id=$2)
		   AND ($3='' OR quota_key=$3)
		 ORDER BY created_at DESC LIMIT 100`,
		sa, userID, featureKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var usages []Usage
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.ID, &u.SourceAccountID, &u.UserID, &u.QuotaKey, &u.UsageAmount, &u.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		usages = append(usages, u)
	}
	if usages == nil {
		usages = []Usage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": usages})
}

// CheckEntitlement checks whether a user has access to a feature and how much quota remains.
func (h *Handlers) CheckEntitlement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID, _ := body["user_id"].(string)
	featureKey, _ := body["feature_key"].(string)
	if featureKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key is required"})
		return
	}

	// Check active subscription
	var subCount int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM entitlement_subscriptions s
		 JOIN entitlement_plans p ON p.id=s.plan_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND s.status='active' AND p.features ? $3`,
		sa, userID, featureKey,
	).Scan(&subCount); err != nil {
		log.Printf("entitlement subCount lookup: %v", err)
	}

	allowed := subCount > 0

	// Calculate remaining quota
	var used int
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(usage_amount),0) FROM entitlement_usage
		 WHERE source_account_id=$1 AND ($2='' OR user_id=$2) AND quota_key=$3`,
		sa, userID, featureKey,
	).Scan(&used); err != nil {
		log.Printf("entitlement used lookup: %v", err)
	}

	var limitValue *int
	if err := h.pool.QueryRow(ctx,
		`SELECT q.limit_value FROM entitlement_quotas q
		 JOIN entitlement_subscriptions s ON s.plan_id=q.plan_id
		 JOIN entitlement_features f ON f.id=q.feature_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND f.key=$3 AND s.status='active'
		 LIMIT 1`,
		sa, userID, featureKey,
	).Scan(&limitValue); err != nil {
		log.Printf("entitlement limitValue lookup: %v", err)
	}

	remaining := -1 // -1 means unlimited
	if limitValue != nil {
		remaining = *limitValue - used
		if remaining < 0 {
			remaining = 0
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":   allowed,
		"remaining": remaining,
	})
}
