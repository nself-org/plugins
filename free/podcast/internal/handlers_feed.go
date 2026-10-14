package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

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
	if desc != nil {
		descStr = xmlEscape(*desc)
	}
	authorStr := ""
	if author != nil {
		authorStr = *author
	}

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
			if epDesc != nil {
				epDescStr = xmlEscape(*epDesc)
			}
			pubDate := ""
			if epPub != nil {
				pubDate = epPub.Format(time.RFC1123Z)
			}
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
	sa := h.sa(r)
	podcastID := chi.URLParam(r, "id")
	var body struct {
		UserID string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	var sub Subscription
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_podcast_subscriptions (podcast_id, source_account_id, user_id) VALUES ($1,$2,$3)
		 ON CONFLICT DO NOTHING
		 RETURNING id, podcast_id, source_account_id, user_id, created_at`,
		podcastID, sa, body.UserID,
	).Scan(&sub.ID, &sub.PodcastID, &sub.SourceAccountID, &sub.UserID, &sub.CreatedAt)
	if err != nil {
		writeJSON(w, 200, map[string]any{"already_subscribed": true})
		return
	}
	h.pool.Exec(r.Context(), `UPDATE np_podcast_podcasts SET subscriber_count=subscriber_count+1 WHERE id=$1`, podcastID)
	writeJSON(w, 201, map[string]any{"subscription": sub})
}

func (h *Handlers) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	podcastID := chi.URLParam(r, "id")
	var body struct {
		UserID string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	h.pool.Exec(r.Context(), `DELETE FROM np_podcast_subscriptions WHERE podcast_id=$1 AND source_account_id=$2 AND user_id=$3`, podcastID, sa, body.UserID)
	h.pool.Exec(r.Context(), `UPDATE np_podcast_podcasts SET subscriber_count=GREATEST(0,subscriber_count-1) WHERE id=$1`, podcastID)
	writeJSON(w, 200, map[string]any{"success": true})
}
