package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "retro-gaming"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// normalizeTitle lowercases and strips non-alphanumeric characters for search normalization.
func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	prevSpace := false
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace && b.Len() > 0 {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// =========================================================================
// ROMs
// =========================================================================

func (h *Handlers) ListROMs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, rom_file_path, rom_file_size_bytes, rom_file_hash,
		        game_title, game_title_normalized, platform, region, release_year,
		        genre, publisher, developer, igdb_id, moby_games_id,
		        box_art_url, box_art_local_path, screenshot_urls, screenshot_local_paths,
		        description, description_source, recommended_core, core_overrides,
		        user_rating, play_count, last_played_at, favorite, scan_source,
		        added_by_user_id, created_at, updated_at
		 FROM np_retrogame_roms WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var items []ROM
	for rows.Next() {
		var m ROM
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.RomFilePath, &m.RomFileSizeBytes, &m.RomFileHash,
			&m.GameTitle, &m.GameTitleNormalized, &m.Platform, &m.Region, &m.ReleaseYear,
			&m.Genre, &m.Publisher, &m.Developer, &m.IgdbID, &m.MobyGamesID,
			&m.BoxArtURL, &m.BoxArtLocalPath, &m.ScreenshotURLs, &m.ScreenshotLocalPaths,
			&m.Description, &m.DescriptionSource, &m.RecommendedCore, &m.CoreOverrides,
			&m.UserRating, &m.PlayCount, &m.LastPlayedAt, &m.Favorite, &m.ScanSource,
			&m.AddedByUserID, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		items = append(items, m)
	}
	if items == nil {
		items = []ROM{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) CreateROM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		RomFilePath      string   `json:"rom_file_path"`
		RomFileSizeBytes *int64   `json:"rom_file_size_bytes"`
		RomFileHash      *string  `json:"rom_file_hash"`
		GameTitle        string   `json:"game_title"`
		Platform         string   `json:"platform"`
		Region           *string  `json:"region"`
		ReleaseYear      *int     `json:"release_year"`
		Genre            *string  `json:"genre"`
		Publisher        *string  `json:"publisher"`
		Developer        *string  `json:"developer"`
		IgdbID           *int     `json:"igdb_id"`
		MobyGamesID      *int     `json:"moby_games_id"`
		BoxArtURL        *string  `json:"box_art_url"`
		BoxArtLocalPath  *string  `json:"box_art_local_path"`
		Description      *string  `json:"description"`
		DescriptionSource *string `json:"description_source"`
		RecommendedCore  *string  `json:"recommended_core"`
		ScanSource       *string  `json:"scan_source"`
		AddedByUserID    *string  `json:"added_by_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	titleNorm := normalizeTitle(body.GameTitle)
	var m ROM
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_retrogame_roms (
			source_account_id, rom_file_path, rom_file_size_bytes, rom_file_hash,
			game_title, game_title_normalized, platform, region, release_year,
			genre, publisher, developer, igdb_id, moby_games_id,
			box_art_url, box_art_local_path, description, description_source,
			recommended_core, scan_source, added_by_user_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING id, source_account_id, rom_file_path, rom_file_size_bytes, rom_file_hash,
		          game_title, game_title_normalized, platform, region, release_year,
		          genre, publisher, developer, igdb_id, moby_games_id,
		          box_art_url, box_art_local_path, screenshot_urls, screenshot_local_paths,
		          description, description_source, recommended_core, core_overrides,
		          user_rating, play_count, last_played_at, favorite, scan_source,
		          added_by_user_id, created_at, updated_at`,
		sa, body.RomFilePath, body.RomFileSizeBytes, body.RomFileHash,
		body.GameTitle, titleNorm, body.Platform, body.Region, body.ReleaseYear,
		body.Genre, body.Publisher, body.Developer, body.IgdbID, body.MobyGamesID,
		body.BoxArtURL, body.BoxArtLocalPath, body.Description, body.DescriptionSource,
		body.RecommendedCore, body.ScanSource, body.AddedByUserID,
	).Scan(
		&m.ID, &m.SourceAccountID, &m.RomFilePath, &m.RomFileSizeBytes, &m.RomFileHash,
		&m.GameTitle, &m.GameTitleNormalized, &m.Platform, &m.Region, &m.ReleaseYear,
		&m.Genre, &m.Publisher, &m.Developer, &m.IgdbID, &m.MobyGamesID,
		&m.BoxArtURL, &m.BoxArtLocalPath, &m.ScreenshotURLs, &m.ScreenshotLocalPaths,
		&m.Description, &m.DescriptionSource, &m.RecommendedCore, &m.CoreOverrides,
		&m.UserRating, &m.PlayCount, &m.LastPlayedAt, &m.Favorite, &m.ScanSource,
		&m.AddedByUserID, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetROM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var m ROM
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, rom_file_path, rom_file_size_bytes, rom_file_hash,
		        game_title, game_title_normalized, platform, region, release_year,
		        genre, publisher, developer, igdb_id, moby_games_id,
		        box_art_url, box_art_local_path, screenshot_urls, screenshot_local_paths,
		        description, description_source, recommended_core, core_overrides,
		        user_rating, play_count, last_played_at, favorite, scan_source,
		        added_by_user_id, created_at, updated_at
		 FROM np_retrogame_roms WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(
		&m.ID, &m.SourceAccountID, &m.RomFilePath, &m.RomFileSizeBytes, &m.RomFileHash,
		&m.GameTitle, &m.GameTitleNormalized, &m.Platform, &m.Region, &m.ReleaseYear,
		&m.Genre, &m.Publisher, &m.Developer, &m.IgdbID, &m.MobyGamesID,
		&m.BoxArtURL, &m.BoxArtLocalPath, &m.ScreenshotURLs, &m.ScreenshotLocalPaths,
		&m.Description, &m.DescriptionSource, &m.RecommendedCore, &m.CoreOverrides,
		&m.UserRating, &m.PlayCount, &m.LastPlayedAt, &m.Favorite, &m.ScanSource,
		&m.AddedByUserID, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdateROM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		GameTitle        *string  `json:"game_title"`
		Platform         *string  `json:"platform"`
		Region           *string  `json:"region"`
		ReleaseYear      *int     `json:"release_year"`
		Genre            *string  `json:"genre"`
		Publisher        *string  `json:"publisher"`
		Developer        *string  `json:"developer"`
		BoxArtURL        *string  `json:"box_art_url"`
		Description      *string  `json:"description"`
		RecommendedCore  *string  `json:"recommended_core"`
		UserRating       *float64 `json:"user_rating"`
		Favorite         *bool    `json:"favorite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_retrogame_roms SET
			game_title = COALESCE($3, game_title),
			game_title_normalized = COALESCE($4, game_title_normalized),
			platform = COALESCE($5, platform),
			region = COALESCE($6, region),
			release_year = COALESCE($7, release_year),
			genre = COALESCE($8, genre),
			publisher = COALESCE($9, publisher),
			developer = COALESCE($10, developer),
			box_art_url = COALESCE($11, box_art_url),
			description = COALESCE($12, description),
			recommended_core = COALESCE($13, recommended_core),
			user_rating = COALESCE($14, user_rating),
			favorite = COALESCE($15, favorite),
			updated_at = NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa,
		body.GameTitle,
		func() *string {
			if body.GameTitle != nil {
				s := normalizeTitle(*body.GameTitle)
				return &s
			}
			return nil
		}(),
		body.Platform, body.Region, body.ReleaseYear,
		body.Genre, body.Publisher, body.Developer,
		body.BoxArtURL, body.Description, body.RecommendedCore,
		body.UserRating, body.Favorite,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.GetROM(w, r)
}

func (h *Handlers) DeleteROM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_retrogame_roms WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// =========================================================================
// Save States
// =========================================================================

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
		UserID                string  `json:"user_id"`
		RomID                 string  `json:"rom_id"`
		Slot                  int     `json:"slot"`
		SaveStateFilePath     string  `json:"save_state_file_path"`
		SaveStateFileSizeBytes *int64 `json:"save_state_file_size_bytes"`
		ScreenshotURL         *string `json:"screenshot_url"`
		ScreenshotLocalPath   *string `json:"screenshot_local_path"`
		EmulatorCore          string  `json:"emulator_core"`
		EmulatorVersion       *string `json:"emulator_version"`
		Description           *string `json:"description"`
		PlayTimeSeconds       *int    `json:"play_time_seconds"`
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
		DisplayName         *string `json:"display_name"`
		CoreWasmPath        *string `json:"core_wasm_path"`
		Version             *string `json:"version"`
		IsRecommended       *bool   `json:"is_recommended"`
		Priority            *int    `json:"priority"`
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
