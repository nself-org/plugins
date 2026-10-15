package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListSaveStates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, user_id, rom_id, source_account_id, slot,
		        save_state_file_path, save_state_file_size_bytes,
		        screenshot_url, screenshot_local_path, emulator_core, emulator_version,
		        description, play_time_seconds, created_at, updated_at
		 FROM np_retrogame_save_states WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []SaveState
	for rows.Next() {
		var m SaveState
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Slot,
			&m.SaveStateFilePath, &m.SaveStateFileSizeBytes,
			&m.ScreenshotURL, &m.ScreenshotLocalPath, &m.EmulatorCore, &m.EmulatorVersion,
			&m.Description, &m.PlayTimeSeconds, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []SaveState{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreateSaveState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		UserID                 string  `json:"user_id"`
		RomID                  string  `json:"rom_id"`
		Slot                   int     `json:"slot"`
		SaveStateFilePath      string  `json:"save_state_file_path"`
		SaveStateFileSizeBytes *int64  `json:"save_state_file_size_bytes"`
		ScreenshotURL          *string `json:"screenshot_url"`
		ScreenshotLocalPath    *string `json:"screenshot_local_path"`
		EmulatorCore           string  `json:"emulator_core"`
		EmulatorVersion        *string `json:"emulator_version"`
		Description            *string `json:"description"`
		PlayTimeSeconds        *int    `json:"play_time_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var m SaveState
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_save_states (
			user_id, rom_id, source_account_id, slot, save_state_file_path,
			save_state_file_size_bytes, screenshot_url, screenshot_local_path,
			emulator_core, emulator_version, description, play_time_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (source_account_id, user_id, rom_id, slot) DO UPDATE SET
			save_state_file_path = EXCLUDED.save_state_file_path,
			save_state_file_size_bytes = EXCLUDED.save_state_file_size_bytes,
			screenshot_url = EXCLUDED.screenshot_url,
			screenshot_local_path = EXCLUDED.screenshot_local_path,
			emulator_core = EXCLUDED.emulator_core,
			emulator_version = EXCLUDED.emulator_version,
			description = EXCLUDED.description,
			play_time_seconds = EXCLUDED.play_time_seconds,
			updated_at = NOW()
		RETURNING id, user_id, rom_id, source_account_id, slot,
		          save_state_file_path, save_state_file_size_bytes,
		          screenshot_url, screenshot_local_path, emulator_core, emulator_version,
		          description, play_time_seconds, created_at, updated_at`,
		body.UserID, body.RomID, sa, body.Slot, body.SaveStateFilePath,
		body.SaveStateFileSizeBytes, body.ScreenshotURL, body.ScreenshotLocalPath,
		body.EmulatorCore, body.EmulatorVersion, body.Description, body.PlayTimeSeconds,
	).Scan(
		&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Slot,
		&m.SaveStateFilePath, &m.SaveStateFileSizeBytes,
		&m.ScreenshotURL, &m.ScreenshotLocalPath, &m.EmulatorCore, &m.EmulatorVersion,
		&m.Description, &m.PlayTimeSeconds, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetSaveState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var m SaveState
	err := h.pool.QueryRow(ctx,
		`SELECT id, user_id, rom_id, source_account_id, slot,
		        save_state_file_path, save_state_file_size_bytes,
		        screenshot_url, screenshot_local_path, emulator_core, emulator_version,
		        description, play_time_seconds, created_at, updated_at
		 FROM np_retrogame_save_states WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&m.ID, &m.UserID, &m.RomID, &m.SourceAccountID, &m.Slot,
		&m.SaveStateFilePath, &m.SaveStateFileSizeBytes,
		&m.ScreenshotURL, &m.ScreenshotLocalPath, &m.EmulatorCore, &m.EmulatorVersion,
		&m.Description, &m.PlayTimeSeconds, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdateSaveState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Description     *string `json:"description"`
		PlayTimeSeconds *int    `json:"play_time_seconds"`
		ScreenshotURL   *string `json:"screenshot_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_retrogame_save_states SET
			description = COALESCE($3, description),
			play_time_seconds = COALESCE($4, play_time_seconds),
			screenshot_url = COALESCE($5, screenshot_url),
			updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body.Description, body.PlayTimeSeconds, body.ScreenshotURL,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.GetSaveState(w, r)
}

func (h *Handlers) DeleteSaveState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_retrogame_save_states WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// =========================================================================
// Play Sessions
// =========================================================================
