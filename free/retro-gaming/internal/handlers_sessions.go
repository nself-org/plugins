package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (h *Handlers) ListPlaySessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, user_id, rom_id, source_account_id, platform, device_id,
		        emulator_core, started_at, ended_at, duration_seconds,
		        save_state_id, auto_save_created, controller_type, created_at
		 FROM np_retrogame_play_sessions WHERE source_account_id=$1 ORDER BY started_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []PlaySession
	for rows.Next() {
		var m PlaySession
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Platform, &m.DeviceID,
			&m.EmulatorCore, &m.StartedAt, &m.EndedAt, &m.DurationSeconds,
			&m.SaveStateID, &m.AutoSaveCreated, &m.ControllerType, &m.CreatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []PlaySession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreatePlaySession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		UserID         string  `json:"user_id"`
		RomID          string  `json:"rom_id"`
		Platform       string  `json:"platform"`
		DeviceID       *string `json:"device_id"`
		EmulatorCore   string  `json:"emulator_core"`
		ControllerType *string `json:"controller_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var m PlaySession
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_play_sessions (
			user_id, rom_id, source_account_id, platform, device_id,
			emulator_core, controller_type
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, user_id, rom_id, source_account_id, platform, device_id,
		          emulator_core, started_at, ended_at, duration_seconds,
		          save_state_id, auto_save_created, controller_type, created_at`,
		body.UserID, body.RomID, sa, body.Platform, body.DeviceID,
		body.EmulatorCore, body.ControllerType,
	).Scan(
		&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Platform, &m.DeviceID,
		&m.EmulatorCore, &m.StartedAt, &m.EndedAt, &m.DurationSeconds,
		&m.SaveStateID, &m.AutoSaveCreated, &m.ControllerType, &m.CreatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetPlaySession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var m PlaySession
	err := h.pool.QueryRow(ctx,
		`SELECT id, user_id, rom_id, source_account_id, platform, device_id,
		        emulator_core, started_at, ended_at, duration_seconds,
		        save_state_id, auto_save_created, controller_type, created_at
		 FROM np_retrogame_play_sessions WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Platform, &m.DeviceID,
		&m.EmulatorCore, &m.StartedAt, &m.EndedAt, &m.DurationSeconds,
		&m.SaveStateID, &m.AutoSaveCreated, &m.ControllerType, &m.CreatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdatePlaySession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		EndedAt         *time.Time `json:"ended_at"`
		DurationSeconds *int       `json:"duration_seconds"`
		SaveStateID     *string    `json:"save_state_id"`
		AutoSaveCreated *bool      `json:"auto_save_created"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_retrogame_play_sessions SET
			ended_at = COALESCE($3, ended_at),
			duration_seconds = COALESCE($4, duration_seconds),
			save_state_id = COALESCE($5, save_state_id),
			auto_save_created = COALESCE($6, auto_save_created)
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body.EndedAt, body.DurationSeconds, body.SaveStateID, body.AutoSaveCreated,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.GetPlaySession(w, r)
}

func (h *Handlers) DeletePlaySession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_retrogame_play_sessions WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// =========================================================================
// Emulator Cores
// =========================================================================
