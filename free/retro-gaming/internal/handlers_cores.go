package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListEmulatorCores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.pool.Query(ctx,
		`SELECT id, core_name, display_name, platform, core_wasm_path, core_wasm_size_bytes,
		        version, license, author, homepage_url,
		        supports_save_states, supports_rewind, supports_fast_forward, supports_cheats,
		        default_config, is_recommended, priority, created_at, updated_at
		 FROM np_retrogame_emulator_cores ORDER BY platform, priority, core_name`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []EmulatorCore
	for rows.Next() {
		var m EmulatorCore
		if err := rows.Scan(
			&m.ID, &m.CoreName, &m.DisplayName, &m.Platform, &m.CoreWasmPath, &m.CoreWasmSizeBytes,
			&m.Version, &m.License, &m.Author, &m.HomepageURL,
			&m.SupportsSaveStates, &m.SupportsRewind, &m.SupportsFastForward, &m.SupportsCheats,
			&m.DefaultConfig, &m.IsRecommended, &m.Priority, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []EmulatorCore{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreateEmulatorCore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body struct {
		CoreName            string  `json:"core_name"`
		DisplayName         string  `json:"display_name"`
		Platform            string  `json:"platform"`
		CoreWasmPath        *string `json:"core_wasm_path"`
		Version             string  `json:"version"`
		License             *string `json:"license"`
		Author              *string `json:"author"`
		HomepageURL         *string `json:"homepage_url"`
		SupportsSaveStates  *bool   `json:"supports_save_states"`
		SupportsRewind      *bool   `json:"supports_rewind"`
		SupportsFastForward *bool   `json:"supports_fast_forward"`
		SupportsCheats      *bool   `json:"supports_cheats"`
		IsRecommended       *bool   `json:"is_recommended"`
		Priority            *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var m EmulatorCore
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_emulator_cores (
			core_name, display_name, platform, core_wasm_path, version,
			license, author, homepage_url, supports_save_states, supports_rewind,
			supports_fast_forward, supports_cheats, is_recommended, priority
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,
			COALESCE($9, true), COALESCE($10, false),
			COALESCE($11, true), COALESCE($12, false),
			COALESCE($13, false), COALESCE($14, 10))
		ON CONFLICT (core_name, platform) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			core_wasm_path = EXCLUDED.core_wasm_path,
			version = EXCLUDED.version,
			license = EXCLUDED.license,
			author = EXCLUDED.author,
			homepage_url = EXCLUDED.homepage_url,
			supports_save_states = EXCLUDED.supports_save_states,
			supports_rewind = EXCLUDED.supports_rewind,
			supports_fast_forward = EXCLUDED.supports_fast_forward,
			supports_cheats = EXCLUDED.supports_cheats,
			is_recommended = EXCLUDED.is_recommended,
			priority = EXCLUDED.priority,
			updated_at = NOW()
		RETURNING id, core_name, display_name, platform, core_wasm_path, core_wasm_size_bytes,
		          version, license, author, homepage_url,
		          supports_save_states, supports_rewind, supports_fast_forward, supports_cheats,
		          default_config, is_recommended, priority, created_at, updated_at`,
		body.CoreName, body.DisplayName, body.Platform, body.CoreWasmPath, body.Version,
		body.License, body.Author, body.HomepageURL,
		body.SupportsSaveStates, body.SupportsRewind, body.SupportsFastForward, body.SupportsCheats,
		body.IsRecommended, body.Priority,
	).Scan(
		&m.ID, &m.CoreName, &m.DisplayName, &m.Platform, &m.CoreWasmPath, &m.CoreWasmSizeBytes,
		&m.Version, &m.License, &m.Author, &m.HomepageURL,
		&m.SupportsSaveStates, &m.SupportsRewind, &m.SupportsFastForward, &m.SupportsCheats,
		&m.DefaultConfig, &m.IsRecommended, &m.Priority, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetEmulatorCore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var m EmulatorCore
	err := h.pool.QueryRow(ctx,
		`SELECT id, core_name, display_name, platform, core_wasm_path, core_wasm_size_bytes,
		        version, license, author, homepage_url,
		        supports_save_states, supports_rewind, supports_fast_forward, supports_cheats,
		        default_config, is_recommended, priority, created_at, updated_at
		 FROM np_retrogame_emulator_cores WHERE id=$1`, id,
	).Scan(
		&m.ID, &m.CoreName, &m.DisplayName, &m.Platform, &m.CoreWasmPath, &m.CoreWasmSizeBytes,
		&m.Version, &m.License, &m.Author, &m.HomepageURL,
		&m.SupportsSaveStates, &m.SupportsRewind, &m.SupportsFastForward, &m.SupportsCheats,
		&m.DefaultConfig, &m.IsRecommended, &m.Priority, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdateEmulatorCore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	var body struct {
		DisplayName   *string `json:"display_name"`
		CoreWasmPath  *string `json:"core_wasm_path"`
		Version       *string `json:"version"`
		IsRecommended *bool   `json:"is_recommended"`
		Priority      *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_retrogame_emulator_cores SET
			display_name = COALESCE($2, display_name),
			core_wasm_path = COALESCE($3, core_wasm_path),
			version = COALESCE($4, version),
			is_recommended = COALESCE($5, is_recommended),
			priority = COALESCE($6, priority),
			updated_at = NOW()
		 WHERE id=$1`,
		id, body.DisplayName, body.CoreWasmPath, body.Version, body.IsRecommended, body.Priority,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.GetEmulatorCore(w, r)
}

func (h *Handlers) DeleteEmulatorCore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx, `DELETE FROM np_retrogame_emulator_cores WHERE id=$1`, id)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// =========================================================================
// Controller Configs
// =========================================================================
