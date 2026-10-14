package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handlers) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	podcastID := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at
		 FROM np_podcast_episodes WHERE podcast_id=$1 AND source_account_id=$2 ORDER BY published_at DESC NULLS LAST`, podcastID, sa)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var result []Episode
	for rows.Next() {
		var e Episode
		var desc *string
		rows.Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
		if desc != nil {
			e.Description = *desc
		}
		result = append(result, e)
	}
	if result == nil {
		result = []Episode{}
	}
	writeJSON(w, 200, map[string]any{"data": result, "total": len(result)})
}

func (h *Handlers) CreateEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	podcastID := chi.URLParam(r, "id")
	var body struct {
		Title       string `json:"title"`
		AudioURL    string `json:"audio_url"`
		Description string `json:"description"`
		Duration    int    `json:"duration_seconds"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	var e Episode
	var desc *string
	if body.Description != "" {
		desc = &body.Description
	}
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_episodes (podcast_id, source_account_id, title, description, audio_url, duration_seconds)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at`,
		podcastID, sa, body.Title, desc, body.AudioURL, body.Duration,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	if desc != nil {
		e.Description = *desc
	}
	writeJSON(w, 201, map[string]any{"episode": e})
}

func (h *Handlers) GetEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var e Episode
	var desc *string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at
		 FROM np_podcast_episodes WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if desc != nil {
		e.Description = *desc
	}
	writeJSON(w, 200, map[string]any{"episode": e})
}

func (h *Handlers) UpdateEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Title       *string `json:"title"`
		IsPublished *bool   `json:"is_published"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	var e Episode
	var desc *string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE np_podcast_episodes SET title=COALESCE($3,title), is_published=COALESCE($4,is_published), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at`,
		id, sa, body.Title, body.IsPublished,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if desc != nil {
		e.Description = *desc
	}
	writeJSON(w, 200, map[string]any{"episode": e})
}

func (h *Handlers) DeleteEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM np_podcast_episodes WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (h *Handlers) RecordPlay(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(), `UPDATE np_podcast_episodes SET play_count=play_count+1 WHERE id=$1 AND source_account_id=$2`, id, sa)
	h.pool.Exec(r.Context(), `INSERT INTO np_podcast_plays (episode_id, source_account_id) VALUES ($1,$2)`, id, sa)
	writeJSON(w, 200, map[string]any{"recorded": true})
}
