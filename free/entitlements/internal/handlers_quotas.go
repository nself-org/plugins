package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

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
