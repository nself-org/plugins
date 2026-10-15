package internal

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
)

// ─── Geofences ───────────────────────────────────────────────────────────────

func (h *Handlers) ListGeofences(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	q := r.URL.Query()

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, name, description, fence_type, center_lat, center_lng,
		        radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		        metadata, created_by, created_at, updated_at
		   FROM geo_geofences
		  WHERE source_account_id=$1 AND deleted_at IS NULL
		  ORDER BY created_at DESC`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	fences := []Geofence{}
	for rows.Next() {
		var f Geofence
		if err := rows.Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
			&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
			&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
			&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		// optional active filter
		if active := q.Get("active"); active != "" {
			want := active == "true"
			if f.Active != want {
				continue
			}
		}
		fences = append(fences, f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": fences, "total": len(fences)})
}

func (h *Handlers) CreateGeofence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string          `json:"name"`
		Description   *string         `json:"description"`
		FenceType     string          `json:"fence_type"`
		CenterLat     *float64        `json:"center_lat"`
		CenterLng     *float64        `json:"center_lng"`
		RadiusMeters  *float64        `json:"radius_meters"`
		Polygon       json.RawMessage `json:"polygon"`
		NotifyOnEnter bool            `json:"notify_on_enter"`
		NotifyOnExit  bool            `json:"notify_on_exit"`
		NotifyURL     *string         `json:"notify_url"`
		Metadata      json.RawMessage `json:"metadata"`
		CreatedBy     *string         `json:"created_by"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.CenterLat == nil || body.CenterLng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, center_lat, and center_lng are required"})
		return
	}
	fenceType := body.FenceType
	if fenceType == "" {
		fenceType = "circle"
	}
	meta := body.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	account := sa(r)

	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO geo_geofences
		   (source_account_id, name, description, fence_type, center_lat, center_lng,
		    radius_meters, polygon, notify_on_enter, notify_on_exit, notify_url, metadata, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, source_account_id, name, description, fence_type, center_lat, center_lng,
		           radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		           metadata, created_by, created_at, updated_at`,
		account, body.Name, body.Description, fenceType, body.CenterLat, body.CenterLng,
		body.RadiusMeters, body.Polygon, body.NotifyOnEnter, body.NotifyOnExit,
		body.NotifyURL, meta, body.CreatedBy,
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handlers) GetGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, name, description, fence_type, center_lat, center_lng,
		        radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		        metadata, created_by, created_at, updated_at
		   FROM geo_geofences
		  WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL`,
		id, account,
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handlers) UpdateGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)

	var body struct {
		Name          *string         `json:"name"`
		Description   *string         `json:"description"`
		CenterLat     *float64        `json:"center_lat"`
		CenterLng     *float64        `json:"center_lng"`
		RadiusMeters  *float64        `json:"radius_meters"`
		Polygon       json.RawMessage `json:"polygon"`
		Active        *bool           `json:"active"`
		NotifyOnEnter *bool           `json:"notify_on_enter"`
		NotifyOnExit  *bool           `json:"notify_on_exit"`
		NotifyURL     *string         `json:"notify_url"`
		Metadata      json.RawMessage `json:"metadata"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`UPDATE geo_geofences SET
		   name          = COALESCE($3, name),
		   description   = COALESCE($4, description),
		   center_lat    = COALESCE($5, center_lat),
		   center_lng    = COALESCE($6, center_lng),
		   radius_meters = COALESCE($7, radius_meters),
		   polygon       = COALESCE($8, polygon),
		   active        = COALESCE($9, active),
		   notify_on_enter = COALESCE($10, notify_on_enter),
		   notify_on_exit  = COALESCE($11, notify_on_exit),
		   notify_url    = COALESCE($12, notify_url),
		   metadata      = COALESCE($13, metadata),
		   updated_at    = NOW()
		 WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL
		 RETURNING id, source_account_id, name, description, fence_type, center_lat, center_lng,
		           radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		           metadata, created_by, created_at, updated_at`,
		id, account, body.Name, body.Description, body.CenterLat, body.CenterLng,
		body.RadiusMeters, nilIfEmpty(body.Polygon), body.Active,
		body.NotifyOnEnter, body.NotifyOnExit, body.NotifyURL, nilIfEmpty(body.Metadata),
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handlers) DeleteGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE geo_geofences SET deleted_at=NOW() WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL`,
		id, account,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// CheckGeofence checks whether a point is inside any active geofences.
func (h *Handlers) CheckGeofence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lat        *float64 `json:"lat"`
		Lng        *float64 `json:"lng"`
		EntityID   string   `json:"entity_id"`
		EntityType string   `json:"entity_type"`
	}
	if err := readJSON(r, &body); err != nil || body.Lat == nil || body.Lng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	account := sa(r)
	entityType := body.EntityType
	if entityType == "" {
		entityType = "user"
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, name, center_lat, center_lng, radius_meters, fence_type
		   FROM geo_geofences
		  WHERE source_account_id=$1 AND active=TRUE AND deleted_at IS NULL`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var results []GeofenceEvalResult
	for rows.Next() {
		var fid, fname, ftype string
		var clat, clng float64
		var radius *float64
		if err := rows.Scan(&fid, &fname, &clat, &clng, &radius, &ftype); err != nil {
			continue
		}
		dist := haversine(*body.Lat, *body.Lng, clat, clng)
		inside := false
		if ftype == "circle" && radius != nil {
			inside = dist <= *radius
		}
		if inside && body.EntityID != "" {
			_, _ = h.DB.Pool.Exec(r.Context(),
				`INSERT INTO geo_geofence_events
				   (source_account_id, geofence_id, event_type, entity_id, entity_type, lat, lng)
				 VALUES ($1,$2,'enter',$3,$4,$5,$6)`,
				account, fid, body.EntityID, entityType, *body.Lat, *body.Lng,
			)
		}
		results = append(results, GeofenceEvalResult{
			GeofenceID:     fid,
			GeofenceName:   fname,
			Inside:         inside,
			DistanceMeters: dist,
		})
	}
	if results == nil {
		results = []GeofenceEvalResult{}
	}
	insideCount := 0
	for _, r := range results {
		if r.Inside {
			insideCount++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results, "inside_count": insideCount})
}
