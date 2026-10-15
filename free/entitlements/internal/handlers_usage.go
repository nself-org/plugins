package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
)

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
	h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM entitlement_subscriptions s
		 JOIN entitlement_plans p ON p.id=s.plan_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND s.status='active' AND p.features ? $3`,
		sa, userID, featureKey,
	).Scan(&subCount)

	allowed := subCount > 0

	// Calculate remaining quota
	var used int
	h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(usage_amount),0) FROM entitlement_usage
		 WHERE source_account_id=$1 AND ($2='' OR user_id=$2) AND quota_key=$3`,
		sa, userID, featureKey,
	).Scan(&used)

	var limitValue *int
	h.pool.QueryRow(ctx,
		`SELECT q.limit_value FROM entitlement_quotas q
		 JOIN entitlement_subscriptions s ON s.plan_id=q.plan_id
		 JOIN entitlement_features f ON f.id=q.feature_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND f.key=$3 AND s.status='active'
		 LIMIT 1`,
		sa, userID, featureKey,
	).Scan(&limitValue)

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

// ============================================================
// Addons
// ============================================================
