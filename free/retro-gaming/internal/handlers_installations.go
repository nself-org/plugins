package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListCoreInstallations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, user_id, source_account_id, device_id, device_platform,
		        core_name, core_version, installed_at, last_used_at
		 FROM np_retrogame_core_installations WHERE source_account_id=$1 ORDER BY installed_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []CoreInstallation
	for rows.Next() {
		var m CoreInstallation
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.SourceAccountID, &m.DeviceID, &m.DevicePlatform,
			&m.CoreName, &m.CoreVersion, &m.InstalledAt, &m.LastUsedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []CoreInstallation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreateCoreInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		UserID         string `json:"user_id"`
		DeviceID       string `json:"device_id"`
		DevicePlatform string `json:"device_platform"`
		CoreName       string `json:"core_name"`
		CoreVersion    string `json:"core_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var m CoreInstallation
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_core_installations (
			user_id, source_account_id, device_id, device_platform, core_name, core_version
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (source_account_id, user_id, device_id, core_name) DO UPDATE SET
			core_version = EXCLUDED.core_version,
			last_used_at = NOW()
		RETURNING id, user_id, source_account_id, device_id, device_platform,
		          core_name, core_version, installed_at, last_used_at`,
		body.UserID, sa, body.DeviceID, body.DevicePlatform, body.CoreName, body.CoreVersion,
	).Scan(
		&m.ID, &m.UserID, &m.SourceAccountID, &m.DeviceID, &m.DevicePlatform,
		&m.CoreName, &m.CoreVersion, &m.InstalledAt, &m.LastUsedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetCoreInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var m CoreInstallation
	err := h.pool.QueryRow(ctx,
		`SELECT id, user_id, source_account_id, device_id, device_platform,
		        core_name, core_version, installed_at, last_used_at
		 FROM np_retrogame_core_installations WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&m.ID, &m.UserID, &m.SourceAccountID, &m.DeviceID, &m.DevicePlatform,
		&m.CoreName, &m.CoreVersion, &m.InstalledAt, &m.LastUsedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) DeleteCoreInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_retrogame_core_installations WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
