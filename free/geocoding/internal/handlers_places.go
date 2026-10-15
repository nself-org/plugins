package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

// ─── Places ──────────────────────────────────────────────────────────────────

func (h *Handlers) ListPlaces(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		        formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at
		   FROM geo_places
		  WHERE source_account_id=$1
		  ORDER BY created_at DESC`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	places := []Place{}
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name,
			&p.Category, &p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website,
			&p.Rating, &p.ReviewCount, &p.Hours, &p.Photos, &p.Metadata,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		places = append(places, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": places, "total": len(places)})
}

func (h *Handlers) CreatePlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider         string          `json:"provider"`
		ProviderPlaceID  string          `json:"provider_place_id"`
		Name             string          `json:"name"`
		Category         *string         `json:"category"`
		Lat              *float64        `json:"lat"`
		Lng              *float64        `json:"lng"`
		FormattedAddress *string         `json:"formatted_address"`
		Phone            *string         `json:"phone"`
		Website          *string         `json:"website"`
		Rating           *float64        `json:"rating"`
		ReviewCount      *int            `json:"review_count"`
		Hours            json.RawMessage `json:"hours"`
		Photos           json.RawMessage `json:"photos"`
		Metadata         json.RawMessage `json:"metadata"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Lat == nil || body.Lng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, lat, and lng are required"})
		return
	}
	if body.Provider == "" {
		body.Provider = "manual"
	}
	if body.ProviderPlaceID == "" {
		body.ProviderPlaceID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
	}
	photos := body.Photos
	if len(photos) == 0 {
		photos = json.RawMessage(`[]`)
	}
	meta := body.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	account := sa(r)

	var p Place
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO geo_places
		   (source_account_id, provider, provider_place_id, name, category, lat, lng,
		    formatted_address, phone, website, rating, review_count, hours, photos, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		           formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at`,
		account, body.Provider, body.ProviderPlaceID, body.Name, body.Category, body.Lat, body.Lng,
		body.FormattedAddress, body.Phone, body.Website, body.Rating, body.ReviewCount,
		body.Hours, photos, meta,
	).Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name, &p.Category,
		&p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website, &p.Rating, &p.ReviewCount,
		&p.Hours, &p.Photos, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetPlace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var p Place
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		        formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at
		   FROM geo_places WHERE id=$1 AND source_account_id=$2`,
		id, account,
	).Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name, &p.Category,
		&p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website, &p.Rating, &p.ReviewCount,
		&p.Hours, &p.Photos, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "place not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeletePlace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM geo_places WHERE id=$1 AND source_account_id=$2`, id, account)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "place not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
