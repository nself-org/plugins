package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	pool Querier
}

func NewHandlers(pool Querier) *Handlers {
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

// ─── Health ──────────────────────────────────────────────────────────────────

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "game-metadata"})
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

// ─── Platforms ────────────────────────────────────────────────────────────────

func (h *Handlers) ListPlatforms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 100
	}

	sql := `SELECT id, source_account_id, name, abbreviation, slug, igdb_id,
		generation, manufacturer, platform_family, category, release_date,
		summary, is_active, sort_order, metadata, created_at, updated_at
		FROM np_game_platforms WHERE source_account_id=$1
		ORDER BY sort_order ASC, name ASC LIMIT $2 OFFSET $3`

	rows, err := h.pool.Query(ctx, sql, sa, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var platforms []Platform
	for rows.Next() {
		var p Platform
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Abbreviation, &p.Slug,
			&p.IgdbID, &p.Generation, &p.Manufacturer, &p.PlatformFamily, &p.Category,
			&p.ReleaseDate, &p.Summary, &p.IsActive, &p.SortOrder, &p.Metadata,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		platforms = append(platforms, p)
	}
	if platforms == nil {
		platforms = []Platform{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": platforms, "total": len(platforms)})
}

func (h *Handlers) CreatePlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		Name           string  `json:"name"`
		Abbreviation   *string `json:"abbreviation"`
		Slug           string  `json:"slug"`
		IgdbID         *int    `json:"igdb_id"`
		Generation     *int    `json:"generation"`
		Manufacturer   *string `json:"manufacturer"`
		PlatformFamily *string `json:"platform_family"`
		Category       *string `json:"category"`
		ReleaseDate    *string `json:"release_date"`
		Summary        *string `json:"summary"`
		IsActive       *bool   `json:"is_active"`
		SortOrder      *int    `json:"sort_order"`
		Metadata       any     `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" || body.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug are required"})
		return
	}

	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	meta := "{}"
	if body.Metadata != nil {
		if b, err := json.Marshal(body.Metadata); err == nil {
			meta = string(b)
		}
	}

	var p Platform
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_game_platforms (source_account_id, name, abbreviation, slug, igdb_id,
			generation, manufacturer, platform_family, category, release_date, summary,
			is_active, sort_order, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, source_account_id, name, abbreviation, slug, igdb_id,
			generation, manufacturer, platform_family, category, release_date,
			summary, is_active, sort_order, metadata, created_at, updated_at`,
		sa, body.Name, body.Abbreviation, body.Slug, body.IgdbID,
		body.Generation, body.Manufacturer, body.PlatformFamily, body.Category,
		body.ReleaseDate, body.Summary, isActive, sortOrder, meta,
	).Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Abbreviation, &p.Slug,
		&p.IgdbID, &p.Generation, &p.Manufacturer, &p.PlatformFamily, &p.Category,
		&p.ReleaseDate, &p.Summary, &p.IsActive, &p.SortOrder, &p.Metadata,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetPlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var p Platform
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, abbreviation, slug, igdb_id,
			generation, manufacturer, platform_family, category, release_date,
			summary, is_active, sort_order, metadata, created_at, updated_at
		FROM np_game_platforms WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Abbreviation, &p.Slug,
		&p.IgdbID, &p.Generation, &p.Manufacturer, &p.PlatformFamily, &p.Category,
		&p.ReleaseDate, &p.Summary, &p.IsActive, &p.SortOrder, &p.Metadata,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "platform not found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeletePlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	ct, err := h.pool.Exec(ctx,
		`DELETE FROM np_game_platforms WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "platform not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Genres ───────────────────────────────────────────────────────────────────

func (h *Handlers) ListGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 100
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, name, slug, igdb_id, description, parent_id,
			is_active, sort_order, metadata, created_at, updated_at
		FROM np_game_genres WHERE source_account_id=$1
		ORDER BY sort_order ASC, name ASC LIMIT $2 OFFSET $3`,
		sa, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var genres []Genre
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.SourceAccountID, &g.Name, &g.Slug, &g.IgdbID,
			&g.Description, &g.ParentID, &g.IsActive, &g.SortOrder, &g.Metadata,
			&g.CreatedAt, &g.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		genres = append(genres, g)
	}
	if genres == nil {
		genres = []Genre{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": genres, "total": len(genres)})
}

func (h *Handlers) CreateGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		IgdbID      *int    `json:"igdb_id"`
		Description *string `json:"description"`
		ParentID    *string `json:"parent_id"`
		IsActive    *bool   `json:"is_active"`
		SortOrder   *int    `json:"sort_order"`
		Metadata    any     `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" || body.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and slug are required"})
		return
	}

	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}
	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	meta := "{}"
	if body.Metadata != nil {
		if b, err := json.Marshal(body.Metadata); err == nil {
			meta = string(b)
		}
	}

	var g Genre
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_game_genres (source_account_id, name, slug, igdb_id, description,
			parent_id, is_active, sort_order, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, source_account_id, name, slug, igdb_id, description, parent_id,
			is_active, sort_order, metadata, created_at, updated_at`,
		sa, body.Name, body.Slug, body.IgdbID, body.Description,
		body.ParentID, isActive, sortOrder, meta,
	).Scan(&g.ID, &g.SourceAccountID, &g.Name, &g.Slug, &g.IgdbID,
		&g.Description, &g.ParentID, &g.IsActive, &g.SortOrder, &g.Metadata,
		&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handlers) GetGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var g Genre
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, slug, igdb_id, description, parent_id,
			is_active, sort_order, metadata, created_at, updated_at
		FROM np_game_genres WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&g.ID, &g.SourceAccountID, &g.Name, &g.Slug, &g.IgdbID,
		&g.Description, &g.ParentID, &g.IsActive, &g.SortOrder, &g.Metadata,
		&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "genre not found"})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) DeleteGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	ct, err := h.pool.Exec(ctx,
		`DELETE FROM np_game_genres WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "genre not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Games ────────────────────────────────────────────────────────────────────

func (h *Handlers) ListGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 100
	}

	args := []any{sa}
	where := "source_account_id=$1"
	paramIdx := 2

	if v := q.Get("platform_id"); v != "" {
		where += fmt.Sprintf(" AND platform_id=$%d", paramIdx)
		args = append(args, v)
		paramIdx++
	}
	if v := q.Get("genre_id"); v != "" {
		where += fmt.Sprintf(" AND genre_id=$%d", paramIdx)
		args = append(args, v)
		paramIdx++
	}
	if v := q.Get("tier"); v != "" {
		where += fmt.Sprintf(" AND tier=$%d", paramIdx)
		args = append(args, v)
		paramIdx++
	}
	if v := q.Get("is_verified"); v != "" {
		where += fmt.Sprintf(" AND is_verified=$%d", paramIdx)
		args = append(args, v == "true")
		paramIdx++
	}

	args = append(args, limit, offset)
	sql := fmt.Sprintf(
		`SELECT id, source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, created_at, updated_at
		FROM np_game_catalog WHERE %s
		ORDER BY title ASC LIMIT $%d OFFSET $%d`,
		where, paramIdx, paramIdx+1)

	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var games []Game
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.SourceAccountID, &g.Title, &g.Slug,
			&g.PlatformID, &g.GenreID, &g.ReleaseDate, &g.Developer, &g.Publisher,
			&g.Description, &g.IgdbID, &g.RomHashMd5, &g.RomHashSha1,
			&g.RomHashSha256, &g.RomHashCrc32, &g.RomFilename, &g.RomSizeBytes,
			&g.Tier, &g.Rating, &g.PlayersMin, &g.PlayersMax, &g.IsVerified,
			&g.Metadata, &g.CreatedAt, &g.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		games = append(games, g)
	}
	if games == nil {
		games = []Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": games, "total": len(games)})
}

func (h *Handlers) CreateGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var body struct {
		Title         string   `json:"title"`
		Slug          string   `json:"slug"`
		PlatformID    *string  `json:"platform_id"`
		GenreID       *string  `json:"genre_id"`
		ReleaseDate   *string  `json:"release_date"`
		Developer     *string  `json:"developer"`
		Publisher     *string  `json:"publisher"`
		Description   *string  `json:"description"`
		IgdbID        *int     `json:"igdb_id"`
		RomHashMd5    *string  `json:"rom_hash_md5"`
		RomHashSha1   *string  `json:"rom_hash_sha1"`
		RomHashSha256 *string  `json:"rom_hash_sha256"`
		RomHashCrc32  *string  `json:"rom_hash_crc32"`
		RomFilename   *string  `json:"rom_filename"`
		RomSizeBytes  *int64   `json:"rom_size_bytes"`
		Tier          *string  `json:"tier"`
		Rating        *float64 `json:"rating"`
		PlayersMin    *int     `json:"players_min"`
		PlayersMax    *int     `json:"players_max"`
		IsVerified    *bool    `json:"is_verified"`
		Metadata      any      `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Title == "" || body.Slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title and slug are required"})
		return
	}

	playersMin := 1
	if body.PlayersMin != nil {
		playersMin = *body.PlayersMin
	}
	playersMax := 1
	if body.PlayersMax != nil {
		playersMax = *body.PlayersMax
	}
	isVerified := false
	if body.IsVerified != nil {
		isVerified = *body.IsVerified
	}
	meta := "{}"
	if body.Metadata != nil {
		if b, err := json.Marshal(body.Metadata); err == nil {
			meta = string(b)
		}
	}

	var g Game
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_game_catalog (source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, search_vector)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,
			to_tsvector('english', $2 || ' ' || COALESCE($8,'') || ' ' || COALESCE($7,'') || ' ' || COALESCE($9,'')))
		RETURNING id, source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, created_at, updated_at`,
		sa, body.Title, body.Slug, body.PlatformID, body.GenreID,
		body.ReleaseDate, body.Developer, body.Publisher, body.Description, body.IgdbID,
		body.RomHashMd5, body.RomHashSha1, body.RomHashSha256, body.RomHashCrc32,
		body.RomFilename, body.RomSizeBytes, body.Tier, body.Rating,
		playersMin, playersMax, isVerified, meta,
	).Scan(&g.ID, &g.SourceAccountID, &g.Title, &g.Slug,
		&g.PlatformID, &g.GenreID, &g.ReleaseDate, &g.Developer, &g.Publisher,
		&g.Description, &g.IgdbID, &g.RomHashMd5, &g.RomHashSha1,
		&g.RomHashSha256, &g.RomHashCrc32, &g.RomFilename, &g.RomSizeBytes,
		&g.Tier, &g.Rating, &g.PlayersMin, &g.PlayersMax, &g.IsVerified,
		&g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handlers) GetGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	var g Game
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, created_at, updated_at
		FROM np_game_catalog WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&g.ID, &g.SourceAccountID, &g.Title, &g.Slug,
		&g.PlatformID, &g.GenreID, &g.ReleaseDate, &g.Developer, &g.Publisher,
		&g.Description, &g.IgdbID, &g.RomHashMd5, &g.RomHashSha1,
		&g.RomHashSha256, &g.RomHashCrc32, &g.RomFilename, &g.RomSizeBytes,
		&g.Tier, &g.Rating, &g.PlayersMin, &g.PlayersMax, &g.IsVerified,
		&g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) DeleteGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	ct, err := h.pool.Exec(ctx,
		`DELETE FROM np_game_catalog WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) SearchGames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()

	query := q.Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter required"})
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 50
	}

	args := []any{sa, query}
	where := "source_account_id=$1 AND search_vector @@ plainto_tsquery('english', $2)"
	paramIdx := 3

	if v := q.Get("platform_id"); v != "" {
		where += fmt.Sprintf(" AND platform_id=$%d", paramIdx)
		args = append(args, v)
		paramIdx++
	}
	if v := q.Get("tier"); v != "" {
		where += fmt.Sprintf(" AND tier=$%d", paramIdx)
		args = append(args, v)
		paramIdx++
	}

	args = append(args, limit)
	sql := fmt.Sprintf(
		`SELECT id, source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, created_at, updated_at
		FROM np_game_catalog WHERE %s ORDER BY title ASC LIMIT $%d`,
		where, paramIdx)

	rows, err := h.pool.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var games []Game
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.SourceAccountID, &g.Title, &g.Slug,
			&g.PlatformID, &g.GenreID, &g.ReleaseDate, &g.Developer, &g.Publisher,
			&g.Description, &g.IgdbID, &g.RomHashMd5, &g.RomHashSha1,
			&g.RomHashSha256, &g.RomHashCrc32, &g.RomFilename, &g.RomSizeBytes,
			&g.Tier, &g.Rating, &g.PlayersMin, &g.PlayersMax, &g.IsVerified,
			&g.Metadata, &g.CreatedAt, &g.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		games = append(games, g)
	}
	if games == nil {
		games = []Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": games, "total": len(games)})
}

func (h *Handlers) LookupByHash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	q := r.URL.Query()

	hash := q.Get("hash")
	hashType := q.Get("type")
	if hash == "" || hashType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hash and type parameters required"})
		return
	}

	columnMap := map[string]string{
		"md5":    "rom_hash_md5",
		"sha1":   "rom_hash_sha1",
		"sha256": "rom_hash_sha256",
		"crc32":  "rom_hash_crc32",
	}
	col, ok := columnMap[hashType]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be md5, sha1, sha256, or crc32"})
		return
	}

	var g Game
	err := h.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT id, source_account_id, title, slug, platform_id, genre_id,
			release_date, developer, publisher, description, igdb_id,
			rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
			rom_filename, rom_size_bytes, tier, rating, players_min, players_max,
			is_verified, metadata, created_at, updated_at
		FROM np_game_catalog WHERE %s=$1 AND source_account_id=$2`, col),
		hash, sa,
	).Scan(&g.ID, &g.SourceAccountID, &g.Title, &g.Slug,
		&g.PlatformID, &g.GenreID, &g.ReleaseDate, &g.Developer, &g.Publisher,
		&g.Description, &g.IgdbID, &g.RomHashMd5, &g.RomHashSha1,
		&g.RomHashSha256, &g.RomHashCrc32, &g.RomFilename, &g.RomSizeBytes,
		&g.Tier, &g.Rating, &g.PlayersMin, &g.PlayersMax, &g.IsVerified,
		&g.Metadata, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// ─── Game Metadata ────────────────────────────────────────────────────────────

func (h *Handlers) UpsertGameMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	gameID := chi.URLParam(r, "game_id")

	var body struct {
		Source                string   `json:"source"`
		IgdbID                *int     `json:"igdb_id"`
		IgdbURL               *string  `json:"igdb_url"`
		Summary               *string  `json:"summary"`
		Storyline             *string  `json:"storyline"`
		TotalRating           *float64 `json:"total_rating"`
		TotalRatingCount      *int     `json:"total_rating_count"`
		AggregatedRating      *float64 `json:"aggregated_rating"`
		AggregatedRatingCount *int     `json:"aggregated_rating_count"`
		FirstReleaseDate      *string  `json:"first_release_date"`
		Genres                []string `json:"genres"`
		Themes                []string `json:"themes"`
		Keywords              []string `json:"keywords"`
		GameModes             []string `json:"game_modes"`
		Franchises            []string `json:"franchises"`
		AlternativeNames      []string `json:"alternative_names"`
		Websites              any      `json:"websites"`
		AgeRatings            any      `json:"age_ratings"`
		InvolvedCompanies     any      `json:"involved_companies"`
		RawData               any      `json:"raw_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Source == "" {
		body.Source = "igdb"
	}

	toJSON := func(v any) string {
		if v == nil {
			return "{}"
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	toJSONArr := func(v any) string {
		if v == nil {
			return "[]"
		}
		b, _ := json.Marshal(v)
		return string(b)
	}

	genres := body.Genres
	if genres == nil {
		genres = []string{}
	}
	themes := body.Themes
	if themes == nil {
		themes = []string{}
	}
	keywords := body.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	gameModes := body.GameModes
	if gameModes == nil {
		gameModes = []string{}
	}
	franchises := body.Franchises
	if franchises == nil {
		franchises = []string{}
	}
	altNames := body.AlternativeNames
	if altNames == nil {
		altNames = []string{}
	}

	var m GameMetadata
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_game_metadata (source_account_id, game_id, source, igdb_id, igdb_url,
			summary, storyline, total_rating, total_rating_count,
			aggregated_rating, aggregated_rating_count, first_release_date,
			genres, themes, keywords, game_modes, franchises, alternative_names,
			websites, age_ratings, involved_companies, raw_data, fetched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,NOW())
		ON CONFLICT (source_account_id, game_id, source) DO UPDATE SET
			igdb_id=EXCLUDED.igdb_id, igdb_url=EXCLUDED.igdb_url,
			summary=EXCLUDED.summary, storyline=EXCLUDED.storyline,
			total_rating=EXCLUDED.total_rating, total_rating_count=EXCLUDED.total_rating_count,
			aggregated_rating=EXCLUDED.aggregated_rating,
			aggregated_rating_count=EXCLUDED.aggregated_rating_count,
			first_release_date=EXCLUDED.first_release_date,
			genres=EXCLUDED.genres, themes=EXCLUDED.themes, keywords=EXCLUDED.keywords,
			game_modes=EXCLUDED.game_modes, franchises=EXCLUDED.franchises,
			alternative_names=EXCLUDED.alternative_names,
			websites=EXCLUDED.websites, age_ratings=EXCLUDED.age_ratings,
			involved_companies=EXCLUDED.involved_companies, raw_data=EXCLUDED.raw_data,
			fetched_at=NOW(), updated_at=NOW()
		RETURNING id, source_account_id, game_id, source, igdb_id, igdb_url,
			summary, storyline, total_rating, total_rating_count,
			aggregated_rating, aggregated_rating_count, first_release_date,
			genres, themes, keywords, game_modes, franchises, alternative_names,
			websites, age_ratings, involved_companies, raw_data, fetched_at, created_at, updated_at`,
		sa, gameID, body.Source, body.IgdbID, body.IgdbURL,
		body.Summary, body.Storyline, body.TotalRating, body.TotalRatingCount,
		body.AggregatedRating, body.AggregatedRatingCount, body.FirstReleaseDate,
		genres, themes, keywords, gameModes, franchises, altNames,
		toJSON(body.Websites), toJSON(body.AgeRatings),
		toJSONArr(body.InvolvedCompanies), toJSON(body.RawData),
	).Scan(&m.ID, &m.SourceAccountID, &m.GameID, &m.Source, &m.IgdbID, &m.IgdbURL,
		&m.Summary, &m.Storyline, &m.TotalRating, &m.TotalRatingCount,
		&m.AggregatedRating, &m.AggregatedRatingCount, &m.FirstReleaseDate,
		&m.Genres, &m.Themes, &m.Keywords, &m.GameModes, &m.Franchises, &m.AlternativeNames,
		&m.Websites, &m.AgeRatings, &m.InvolvedCompanies, &m.RawData,
		&m.FetchedAt, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("upsert: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) GetGameMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	gameID := chi.URLParam(r, "game_id")
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "igdb"
	}

	var m GameMetadata
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, game_id, source, igdb_id, igdb_url,
			summary, storyline, total_rating, total_rating_count,
			aggregated_rating, aggregated_rating_count, first_release_date,
			genres, themes, keywords, game_modes, franchises, alternative_names,
			websites, age_ratings, involved_companies, raw_data, fetched_at, created_at, updated_at
		FROM np_game_metadata WHERE game_id=$1 AND source=$2 AND source_account_id=$3`,
		gameID, source, sa,
	).Scan(&m.ID, &m.SourceAccountID, &m.GameID, &m.Source, &m.IgdbID, &m.IgdbURL,
		&m.Summary, &m.Storyline, &m.TotalRating, &m.TotalRatingCount,
		&m.AggregatedRating, &m.AggregatedRatingCount, &m.FirstReleaseDate,
		&m.Genres, &m.Themes, &m.Keywords, &m.GameModes, &m.Franchises, &m.AlternativeNames,
		&m.Websites, &m.AgeRatings, &m.InvolvedCompanies, &m.RawData,
		&m.FetchedAt, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "metadata not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ─── Artwork ──────────────────────────────────────────────────────────────────

func (h *Handlers) ListArtwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	gameID := chi.URLParam(r, "game_id")

	args := []any{gameID, sa}
	where := "game_id=$1 AND source_account_id=$2"
	paramIdx := 3

	if v := r.URL.Query().Get("type"); v != "" {
		where += fmt.Sprintf(" AND artwork_type=$%d", paramIdx)
		args = append(args, v)
	}

	rows, err := h.pool.Query(ctx,
		fmt.Sprintf(`SELECT id, source_account_id, game_id, artwork_type, url, local_path,
			width, height, mime_type, file_size_bytes, source, igdb_image_id,
			is_primary, sort_order, metadata, created_at, updated_at
		FROM np_game_artwork WHERE %s
		ORDER BY is_primary DESC, sort_order ASC`, where),
		args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()

	var artworks []Artwork
	for rows.Next() {
		var a Artwork
		if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.GameID, &a.ArtworkType,
			&a.URL, &a.LocalPath, &a.Width, &a.Height, &a.MimeType, &a.FileSizeBytes,
			&a.Source, &a.IgdbImageID, &a.IsPrimary, &a.SortOrder, &a.Metadata,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		artworks = append(artworks, a)
	}
	if artworks == nil {
		artworks = []Artwork{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": artworks, "total": len(artworks)})
}

func (h *Handlers) CreateArtwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	gameID := chi.URLParam(r, "game_id")

	var body struct {
		ArtworkType   string  `json:"artwork_type"`
		URL           *string `json:"url"`
		LocalPath     *string `json:"local_path"`
		Width         *int    `json:"width"`
		Height        *int    `json:"height"`
		MimeType      *string `json:"mime_type"`
		FileSizeBytes *int64  `json:"file_size_bytes"`
		Source        string  `json:"source"`
		IgdbImageID   *string `json:"igdb_image_id"`
		IsPrimary     *bool   `json:"is_primary"`
		SortOrder     *int    `json:"sort_order"`
		Metadata      any     `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.ArtworkType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "artwork_type is required"})
		return
	}
	if body.Source == "" {
		body.Source = "igdb"
	}

	isPrimary := false
	if body.IsPrimary != nil {
		isPrimary = *body.IsPrimary
	}
	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	meta := "{}"
	if body.Metadata != nil {
		if b, err := json.Marshal(body.Metadata); err == nil {
			meta = string(b)
		}
	}

	var a Artwork
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_game_artwork (source_account_id, game_id, artwork_type, url, local_path,
			width, height, mime_type, file_size_bytes, source, igdb_image_id,
			is_primary, sort_order, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, source_account_id, game_id, artwork_type, url, local_path,
			width, height, mime_type, file_size_bytes, source, igdb_image_id,
			is_primary, sort_order, metadata, created_at, updated_at`,
		sa, gameID, body.ArtworkType, body.URL, body.LocalPath,
		body.Width, body.Height, body.MimeType, body.FileSizeBytes, body.Source,
		body.IgdbImageID, isPrimary, sortOrder, meta,
	).Scan(&a.ID, &a.SourceAccountID, &a.GameID, &a.ArtworkType,
		&a.URL, &a.LocalPath, &a.Width, &a.Height, &a.MimeType, &a.FileSizeBytes,
		&a.Source, &a.IgdbImageID, &a.IsPrimary, &a.SortOrder, &a.Metadata,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handlers) DeleteArtwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	ct, err := h.pool.Exec(ctx,
		`DELETE FROM np_game_artwork WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "artwork not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	var s Stats
	err := h.pool.QueryRow(ctx,
		`SELECT
			(SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id=$1 AND is_verified=true),
			(SELECT COUNT(*) FROM np_game_platforms WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM np_game_genres WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM np_game_artwork WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM np_game_metadata WHERE source_account_id=$1),
			(SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id=$1 AND igdb_id IS NOT NULL),
			(SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id=$1 AND (rom_hash_md5 IS NOT NULL OR rom_hash_sha1 IS NOT NULL OR rom_hash_crc32 IS NOT NULL))`,
		sa,
	).Scan(&s.TotalGames, &s.VerifiedGames, &s.TotalPlatforms, &s.TotalGenres,
		&s.TotalArtwork, &s.TotalMetadata, &s.GamesWithIgdb, &s.GamesWithHashes)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("stats: %v", err)})
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT COALESCE(tier,'untiered') as tier, COUNT(*) as cnt
		FROM np_game_catalog WHERE source_account_id=$1
		GROUP BY tier ORDER BY tier ASC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("tier stats: %v", err)})
		return
	}
	defer rows.Close()

	s.TierBreakdown = map[string]int{}
	for rows.Next() {
		var tier string
		var cnt int
		if err := rows.Scan(&tier, &cnt); err == nil {
			s.TierBreakdown[tier] = cnt
		}
	}

	writeJSON(w, http.StatusOK, s)
}
