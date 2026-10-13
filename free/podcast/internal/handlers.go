package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct{ pool *pgxpool.Pool }

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

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

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, 503, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (h *Handlers) ListPodcasts(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at
		 FROM np_podcast_podcasts WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var result []Podcast
	for rows.Next() {
		var p Podcast
		var desc, author, imageURL *string
		rows.Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
		if desc != nil {
			p.Description = *desc
		}
		if author != nil {
			p.Author = *author
		}
		if imageURL != nil {
			p.ImageURL = *imageURL
		}
		result = append(result, p)
	}
	if result == nil {
		result = []Podcast{}
	}
	writeJSON(w, 200, map[string]any{"data": result, "total": len(result)})
}

func (h *Handlers) CreatePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Language    string `json:"language"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	if body.Language == "" {
		body.Language = "en"
	}
	var p Podcast
	var desc, author, imageURL *string
	if body.Description != "" {
		desc = &body.Description
	}
	if body.Author != "" {
		author = &body.Author
	}
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_podcasts (source_account_id, title, description, author, language) VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at`,
		sa, body.Title, desc, author, body.Language,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	if desc != nil {
		p.Description = *desc
	}
	if author != nil {
		p.Author = *author
	}
	writeJSON(w, 201, map[string]any{"podcast": p})
}

func (h *Handlers) GetPodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var p Podcast
	var desc, author, imageURL *string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at
		 FROM np_podcast_podcasts WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if desc != nil {
		p.Description = *desc
	}
	if author != nil {
		p.Author = *author
	}
	if imageURL != nil {
		p.ImageURL = *imageURL
	}
	writeJSON(w, 200, map[string]any{"podcast": p})
}

func (h *Handlers) UpdatePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Title    *string `json:"title"`
		IsPublic *bool   `json:"is_public"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	var p Podcast
	var desc, author, imageURL *string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE np_podcast_podcasts SET title=COALESCE($3,title), is_public=COALESCE($4,is_public), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at`,
		id, sa, body.Title, body.IsPublic,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if desc != nil {
		p.Description = *desc
	}
	if author != nil {
		p.Author = *author
	}
	writeJSON(w, 200, map[string]any{"podcast": p})
}

func (h *Handlers) DeletePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM np_podcast_podcasts WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
