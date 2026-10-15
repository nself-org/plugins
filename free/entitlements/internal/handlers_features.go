package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

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

// ============================================================
// Quotas
// ============================================================
