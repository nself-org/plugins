package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
)

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
		RomFilePath       string  `json:"rom_file_path"`
		RomFileSizeBytes  *int64  `json:"rom_file_size_bytes"`
		RomFileHash       *string `json:"rom_file_hash"`
		GameTitle         string  `json:"game_title"`
		Platform          string  `json:"platform"`
		Region            *string `json:"region"`
		ReleaseYear       *int    `json:"release_year"`
		Genre             *string `json:"genre"`
		Publisher         *string `json:"publisher"`
		Developer         *string `json:"developer"`
		IgdbID            *int    `json:"igdb_id"`
		MobyGamesID       *int    `json:"moby_games_id"`
		BoxArtURL         *string `json:"box_art_url"`
		BoxArtLocalPath   *string `json:"box_art_local_path"`
		Description       *string `json:"description"`
		DescriptionSource *string `json:"description_source"`
		RecommendedCore   *string `json:"recommended_core"`
		ScanSource        *string `json:"scan_source"`
		AddedByUserID     *string `json:"added_by_user_id"`
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
		GameTitle       *string  `json:"game_title"`
		Platform        *string  `json:"platform"`
		Region          *string  `json:"region"`
		ReleaseYear     *int     `json:"release_year"`
		Genre           *string  `json:"genre"`
		Publisher       *string  `json:"publisher"`
		Developer       *string  `json:"developer"`
		BoxArtURL       *string  `json:"box_art_url"`
		Description     *string  `json:"description"`
		RecommendedCore *string  `json:"recommended_core"`
		UserRating      *float64 `json:"user_rating"`
		Favorite        *bool    `json:"favorite"`
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
