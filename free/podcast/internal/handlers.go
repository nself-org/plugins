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

type Handlers struct{ pool *pgxpool.Pool }

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" { return v }
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok", "plugin": "podcast"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, 503, map[string]string{"status": "not_ready"}); return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (h *Handlers) ListPodcasts(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at
		 FROM np_podcast_podcasts WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil { writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("query: %v", err)}); return }
	defer rows.Close()
	var result []Podcast
	for rows.Next() {
		var p Podcast; var desc, author, imageURL *string
		rows.Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
		if desc != nil { p.Description = *desc }
		if author != nil { p.Author = *author }
		if imageURL != nil { p.ImageURL = *imageURL }
		result = append(result, p)
	}
	if result == nil { result = []Podcast{} }
	writeJSON(w, 200, map[string]any{"data": result, "total": len(result)})
}

func (h *Handlers) CreatePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body struct { Title string `json:"title"`; Description string `json:"description"`; Author string `json:"author"`; Language string `json:"language"` }
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" { writeJSON(w, 400, map[string]string{"error": "title required"}); return }
	if body.Language == "" { body.Language = "en" }
	var p Podcast; var desc, author, imageURL *string
	if body.Description != "" { desc = &body.Description }
	if body.Author != "" { author = &body.Author }
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_podcasts (source_account_id, title, description, author, language) VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at`,
		sa, body.Title, desc, author, body.Language,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("insert: %v", err)}); return }
	if desc != nil { p.Description = *desc }
	if author != nil { p.Author = *author }
	writeJSON(w, 201, map[string]any{"podcast": p})
}

func (h *Handlers) GetPodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	var p Podcast; var desc, author, imageURL *string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at
		 FROM np_podcast_podcasts WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	if desc != nil { p.Description = *desc }
	if author != nil { p.Author = *author }
	if imageURL != nil { p.ImageURL = *imageURL }
	writeJSON(w, 200, map[string]any{"podcast": p})
}

func (h *Handlers) UpdatePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	var body struct { Title *string `json:"title"`; IsPublic *bool `json:"is_public"` }
	json.NewDecoder(r.Body).Decode(&body)
	var p Podcast; var desc, author, imageURL *string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE np_podcast_podcasts SET title=COALESCE($3,title), is_public=COALESCE($4,is_public), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, title, description, author, image_url, language, is_public, episode_count, subscriber_count, created_at, updated_at`,
		id, sa, body.Title, body.IsPublic,
	).Scan(&p.ID, &p.SourceAccountID, &p.Title, &desc, &author, &imageURL, &p.Language, &p.IsPublic, &p.EpisodeCount, &p.SubscriberCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	if desc != nil { p.Description = *desc }
	if author != nil { p.Author = *author }
	writeJSON(w, 200, map[string]any{"podcast": p})
}

func (h *Handlers) DeletePodcast(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM np_podcast_podcasts WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	writeJSON(w, 200, map[string]any{"success": true})
}

func (h *Handlers) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); podcastID := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at
		 FROM np_podcast_episodes WHERE podcast_id=$1 AND source_account_id=$2 ORDER BY published_at DESC NULLS LAST`, podcastID, sa)
	if err != nil { writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("query: %v", err)}); return }
	defer rows.Close()
	var result []Episode
	for rows.Next() {
		var e Episode; var desc *string
		rows.Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
		if desc != nil { e.Description = *desc }
		result = append(result, e)
	}
	if result == nil { result = []Episode{} }
	writeJSON(w, 200, map[string]any{"data": result, "total": len(result)})
}

func (h *Handlers) CreateEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); podcastID := chi.URLParam(r, "id")
	var body struct { Title string `json:"title"`; AudioURL string `json:"audio_url"`; Description string `json:"description"`; Duration int `json:"duration_seconds"` }
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" { writeJSON(w, 400, map[string]string{"error": "title required"}); return }
	var e Episode; var desc *string
	if body.Description != "" { desc = &body.Description }
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_episodes (podcast_id, source_account_id, title, description, audio_url, duration_seconds)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at`,
		podcastID, sa, body.Title, desc, body.AudioURL, body.Duration,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil { writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("insert: %v", err)}); return }
	if desc != nil { e.Description = *desc }
	writeJSON(w, 201, map[string]any{"episode": e})
}

func (h *Handlers) GetEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	var e Episode; var desc *string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at
		 FROM np_podcast_episodes WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	if desc != nil { e.Description = *desc }
	writeJSON(w, 200, map[string]any{"episode": e})
}

func (h *Handlers) UpdateEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	var body struct { Title *string `json:"title"`; IsPublished *bool `json:"is_published"` }
	json.NewDecoder(r.Body).Decode(&body)
	var e Episode; var desc *string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE np_podcast_episodes SET title=COALESCE($3,title), is_published=COALESCE($4,is_published), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, podcast_id, source_account_id, title, description, audio_url, duration_seconds, season, episode_number, is_published, play_count, published_at, created_at, updated_at`,
		id, sa, body.Title, body.IsPublished,
	).Scan(&e.ID, &e.PodcastID, &e.SourceAccountID, &e.Title, &desc, &e.AudioURL, &e.Duration, &e.Season, &e.Episode, &e.IsPublished, &e.PlayCount, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	if desc != nil { e.Description = *desc }
	writeJSON(w, 200, map[string]any{"episode": e})
}

func (h *Handlers) DeleteEpisode(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM np_podcast_episodes WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil || tag.RowsAffected() == 0 { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	writeJSON(w, 200, map[string]any{"success": true})
}

func (h *Handlers) RecordPlay(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(), `UPDATE np_podcast_episodes SET play_count=play_count+1 WHERE id=$1 AND source_account_id=$2`, id, sa)
	h.pool.Exec(r.Context(), `INSERT INTO np_podcast_plays (episode_id, source_account_id) VALUES ($1,$2)`, id, sa)
	writeJSON(w, 200, map[string]any{"recorded": true})
}

func (h *Handlers) GetFeed(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	id := chi.URLParam(r, "id")

	// Fetch podcast
	var title, language string
	var desc, author, imageURL *string
	err := h.pool.QueryRow(r.Context(),
		`SELECT title, description, author, image_url, language FROM np_podcast_podcasts WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&title, &desc, &author, &imageURL, &language)
	if err != nil {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(404)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Not Found</title></channel></rss>`)
		return
	}

	// Fetch published episodes
	rows, err := h.pool.Query(r.Context(),
		`SELECT title, description, audio_url, duration_seconds, published_at
		 FROM np_podcast_episodes WHERE podcast_id=$1 AND source_account_id=$2 AND is_published=true
		 ORDER BY published_at DESC NULLS LAST`, id, sa)

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(200)

	descStr := ""
	if desc != nil { descStr = xmlEscape(*desc) }
	authorStr := ""
	if author != nil { authorStr = *author }

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
<channel>
<title>%s</title>
<description>%s</description>
<language>%s</language>
<itunes:author>%s</itunes:author>`, xmlEscape(title), descStr, language, xmlEscape(authorStr))

	if imageURL != nil {
		fmt.Fprintf(w, "\n<itunes:image href=\"%s\"/>", xmlEscape(*imageURL))
	}

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var epTitle string
			var epDesc *string
			var epAudio string
			var epDuration int
			var epPub *time.Time
			rows.Scan(&epTitle, &epDesc, &epAudio, &epDuration, &epPub)
			epDescStr := ""
			if epDesc != nil { epDescStr = xmlEscape(*epDesc) }
			pubDate := ""
			if epPub != nil { pubDate = epPub.Format(time.RFC1123Z) }
			fmt.Fprintf(w, `
<item>
<title>%s</title>
<description>%s</description>
<enclosure url="%s" type="audio/mpeg"/>
<itunes:duration>%d</itunes:duration>
<pubDate>%s</pubDate>
</item>`, xmlEscape(epTitle), epDescStr, xmlEscape(epAudio), epDuration, pubDate)
		}
	}

	fmt.Fprintf(w, "\n</channel>\n</rss>")
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func (h *Handlers) Subscribe(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); podcastID := chi.URLParam(r, "id")
	var body struct { UserID string `json:"user_id"` }
	json.NewDecoder(r.Body).Decode(&body)
	var sub Subscription
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_subscriptions (podcast_id, source_account_id, user_id) VALUES ($1,$2,$3)
		 ON CONFLICT DO NOTHING
		 RETURNING id, podcast_id, source_account_id, user_id, created_at`,
		podcastID, sa, body.UserID,
	).Scan(&sub.ID, &sub.PodcastID, &sub.SourceAccountID, &sub.UserID, &sub.CreatedAt)
	if err != nil {
		writeJSON(w, 200, map[string]any{"already_subscribed": true}); return
	}
	h.pool.Exec(r.Context(), `UPDATE np_podcast_podcasts SET subscriber_count=subscriber_count+1 WHERE id=$1`, podcastID)
	writeJSON(w, 201, map[string]any{"subscription": sub})
}

func (h *Handlers) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r); podcastID := chi.URLParam(r, "id")
	var body struct { UserID string `json:"user_id"` }
	json.NewDecoder(r.Body).Decode(&body)
	h.pool.Exec(r.Context(), `DELETE FROM np_podcast_subscriptions WHERE podcast_id=$1 AND source_account_id=$2 AND user_id=$3`, podcastID, sa, body.UserID)
	h.pool.Exec(r.Context(), `UPDATE np_podcast_podcasts SET subscriber_count=GREATEST(0,subscriber_count-1) WHERE id=$1`, podcastID)
	writeJSON(w, 200, map[string]any{"success": true})
}
