package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListControllerConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, config_name, platform, controller_type,
		        button_mapping, touch_layout, analog_sensitivity, vibration_enabled,
		        created_at, updated_at
		 FROM np_retrogame_controller_configs WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []ControllerConfig
	for rows.Next() {
		var m ControllerConfig
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.UserID, &m.ConfigName, &m.Platform, &m.ControllerType,
			&m.ButtonMapping, &m.TouchLayout, &m.AnalogSensitivity, &m.VibrationEnabled,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []ControllerConfig{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreateControllerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		UserID            string   `json:"user_id"`
		ConfigName        string   `json:"config_name"`
		Platform          *string  `json:"platform"`
		ControllerType    string   `json:"controller_type"`
		ButtonMapping     string   `json:"button_mapping"`
		TouchLayout       *string  `json:"touch_layout"`
		AnalogSensitivity *float64 `json:"analog_sensitivity"`
		VibrationEnabled  *bool    `json:"vibration_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var m ControllerConfig
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_controller_configs (
			source_account_id, user_id, config_name, platform, controller_type,
			button_mapping, touch_layout, analog_sensitivity, vibration_enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,COALESCE($8,1.0),COALESCE($9,true))
		RETURNING id, source_account_id, user_id, config_name, platform, controller_type,
		          button_mapping, touch_layout, analog_sensitivity, vibration_enabled,
		          created_at, updated_at`,
		sa, body.UserID, body.ConfigName, body.Platform, body.ControllerType,
		body.ButtonMapping, body.TouchLayout, body.AnalogSensitivity, body.VibrationEnabled,
	).Scan(
		&m.ID, &m.SourceAccountID, &m.UserID, &m.ConfigName, &m.Platform, &m.ControllerType,
		&m.ButtonMapping, &m.TouchLayout, &m.AnalogSensitivity, &m.VibrationEnabled,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetControllerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var m ControllerConfig
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, user_id, config_name, platform, controller_type,
		        button_mapping, touch_layout, analog_sensitivity, vibration_enabled,
		        created_at, updated_at
		 FROM np_retrogame_controller_configs WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&m.ID, &m.SourceAccountID, &m.UserID, &m.ConfigName, &m.Platform, &m.ControllerType,
		&m.ButtonMapping, &m.TouchLayout, &m.AnalogSensitivity, &m.VibrationEnabled,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdateControllerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		ConfigName        *string  `json:"config_name"`
		ButtonMapping     *string  `json:"button_mapping"`
		TouchLayout       *string  `json:"touch_layout"`
		AnalogSensitivity *float64 `json:"analog_sensitivity"`
		VibrationEnabled  *bool    `json:"vibration_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_retrogame_controller_configs SET
			config_name = COALESCE($3, config_name),
			button_mapping = COALESCE($4, button_mapping),
			touch_layout = COALESCE($5, touch_layout),
			analog_sensitivity = COALESCE($6, analog_sensitivity),
			vibration_enabled = COALESCE($7, vibration_enabled),
			updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body.ConfigName, body.ButtonMapping, body.TouchLayout,
		body.AnalogSensitivity, body.VibrationEnabled,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.GetControllerConfig(w, r)
}

func (h *Handlers) DeleteControllerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_retrogame_controller_configs WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// =========================================================================
// Core Installations
// =========================================================================
