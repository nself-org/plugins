package internal

import (
	"encoding/json"
	"net/http"

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

// Health / Ready

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.pool.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// =============================================================================
// Calendars
// =============================================================================

func (h *Handlers) ListCalendars(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, color, owner_id, is_default, is_public, metadata
		 FROM meetings_calendars WHERE source_account_id = $1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var c Calendar
		var meta []byte
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.CreatedAt, &c.UpdatedAt,
			&c.Name, &c.Description, &c.Color, &c.OwnerID, &c.IsDefault, &c.IsPublic, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		json.Unmarshal(meta, &c.Metadata)
		items = append(items, map[string]any{"calendar": c})
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendars": items, "total": len(items)})
}

func (h *Handlers) CreateCalendar(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_calendars (source_account_id, name, description, color, owner_id, is_default, is_public, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, source_account_id, created_at, updated_at, name, description, color, owner_id, is_default, is_public, metadata`,
		sa, body["name"], body["description"], body["color"], body["owner_id"],
		body["is_default"], body["is_public"], mustJSON(body["metadata"]))
	var c Calendar
	var meta []byte
	if err := row.Scan(&c.ID, &c.SourceAccountID, &c.CreatedAt, &c.UpdatedAt,
		&c.Name, &c.Description, &c.Color, &c.OwnerID, &c.IsDefault, &c.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	json.Unmarshal(meta, &c.Metadata)
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handlers) GetCalendar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	row := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, color, owner_id, is_default, is_public, metadata
		 FROM meetings_calendars WHERE id = $1 AND source_account_id = $2`, id, sa)
	var c Calendar
	var meta []byte
	if err := row.Scan(&c.ID, &c.SourceAccountID, &c.CreatedAt, &c.UpdatedAt,
		&c.Name, &c.Description, &c.Color, &c.OwnerID, &c.IsDefault, &c.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(meta, &c.Metadata)
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) UpdateCalendar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`UPDATE meetings_calendars SET name=COALESCE($3,name), description=$4, color=$5, is_default=COALESCE($6,is_default),
		 is_public=COALESCE($7,is_public), metadata=COALESCE($8,metadata), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, created_at, updated_at, name, description, color, owner_id, is_default, is_public, metadata`,
		id, sa, body["name"], body["description"], body["color"], body["is_default"], body["is_public"], mustJSON(body["metadata"]))
	var c Calendar
	var meta []byte
	if err := row.Scan(&c.ID, &c.SourceAccountID, &c.CreatedAt, &c.UpdatedAt,
		&c.Name, &c.Description, &c.Color, &c.OwnerID, &c.IsDefault, &c.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(meta, &c.Metadata)
	writeJSON(w, http.StatusOK, c)
}

func (h *Handlers) DeleteCalendar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	_, err := h.pool.Exec(r.Context(),
		`DELETE FROM meetings_calendars WHERE id = $1 AND source_account_id = $2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Rooms
// =============================================================================

func (h *Handlers) ListRooms(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, location, floor, building,
		 capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
		 amenities, is_active, booking_buffer_minutes, is_public, allowed_groups, metadata
		 FROM meetings_rooms WHERE source_account_id = $1 ORDER BY name`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []Room
	for rows.Next() {
		var rm Room
		var amenities, allowedGroups, meta []byte
		if err := rows.Scan(&rm.ID, &rm.SourceAccountID, &rm.CreatedAt, &rm.UpdatedAt,
			&rm.Name, &rm.Description, &rm.Location, &rm.Floor, &rm.Building,
			&rm.Capacity, &rm.HasVideoConference, &rm.HasProjector, &rm.HasWhiteboard, &rm.HasPhone,
			&amenities, &rm.IsActive, &rm.BookingBufferMinutes, &rm.IsPublic, &allowedGroups, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		json.Unmarshal(amenities, &rm.Amenities)
		json.Unmarshal(allowedGroups, &rm.AllowedGroups)
		json.Unmarshal(meta, &rm.Metadata)
		items = append(items, rm)
	}
	if items == nil {
		items = []Room{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": items, "total": len(items)})
}

func (h *Handlers) CreateRoom(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_rooms (source_account_id, name, description, location, floor, building,
		 capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
		 amenities, is_active, booking_buffer_minutes, is_public, allowed_groups, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		 RETURNING id, source_account_id, created_at, updated_at, name, description, location, floor, building,
		 capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
		 amenities, is_active, booking_buffer_minutes, is_public, allowed_groups, metadata`,
		sa, body["name"], body["description"], body["location"], body["floor"], body["building"],
		body["capacity"], body["has_video_conference"], body["has_projector"], body["has_whiteboard"], body["has_phone"],
		mustJSON(body["amenities"]), body["is_active"], body["booking_buffer_minutes"], body["is_public"],
		mustJSON(body["allowed_groups"]), mustJSON(body["metadata"]))
	var rm Room
	var amenities, allowedGroups, meta []byte
	if err := row.Scan(&rm.ID, &rm.SourceAccountID, &rm.CreatedAt, &rm.UpdatedAt,
		&rm.Name, &rm.Description, &rm.Location, &rm.Floor, &rm.Building,
		&rm.Capacity, &rm.HasVideoConference, &rm.HasProjector, &rm.HasWhiteboard, &rm.HasPhone,
		&amenities, &rm.IsActive, &rm.BookingBufferMinutes, &rm.IsPublic, &allowedGroups, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	json.Unmarshal(amenities, &rm.Amenities)
	json.Unmarshal(allowedGroups, &rm.AllowedGroups)
	json.Unmarshal(meta, &rm.Metadata)
	writeJSON(w, http.StatusCreated, rm)
}

func (h *Handlers) GetRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	row := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, location, floor, building,
		 capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
		 amenities, is_active, booking_buffer_minutes, is_public, allowed_groups, metadata
		 FROM meetings_rooms WHERE id=$1 AND source_account_id=$2`, id, sa)
	var rm Room
	var amenities, allowedGroups, meta []byte
	if err := row.Scan(&rm.ID, &rm.SourceAccountID, &rm.CreatedAt, &rm.UpdatedAt,
		&rm.Name, &rm.Description, &rm.Location, &rm.Floor, &rm.Building,
		&rm.Capacity, &rm.HasVideoConference, &rm.HasProjector, &rm.HasWhiteboard, &rm.HasPhone,
		&amenities, &rm.IsActive, &rm.BookingBufferMinutes, &rm.IsPublic, &allowedGroups, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(amenities, &rm.Amenities)
	json.Unmarshal(allowedGroups, &rm.AllowedGroups)
	json.Unmarshal(meta, &rm.Metadata)
	writeJSON(w, http.StatusOK, rm)
}

func (h *Handlers) UpdateRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`UPDATE meetings_rooms SET name=COALESCE($3,name), description=$4, location=$5, floor=$6, building=$7,
		 capacity=COALESCE($8,capacity), has_video_conference=COALESCE($9,has_video_conference),
		 has_projector=COALESCE($10,has_projector), has_whiteboard=COALESCE($11,has_whiteboard),
		 has_phone=COALESCE($12,has_phone), is_active=COALESCE($13,is_active),
		 booking_buffer_minutes=COALESCE($14,booking_buffer_minutes), is_public=COALESCE($15,is_public),
		 updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, created_at, updated_at, name, description, location, floor, building,
		 capacity, has_video_conference, has_projector, has_whiteboard, has_phone,
		 amenities, is_active, booking_buffer_minutes, is_public, allowed_groups, metadata`,
		id, sa, body["name"], body["description"], body["location"], body["floor"], body["building"],
		body["capacity"], body["has_video_conference"], body["has_projector"], body["has_whiteboard"],
		body["has_phone"], body["is_active"], body["booking_buffer_minutes"], body["is_public"])
	var rm Room
	var amenities, allowedGroups, meta []byte
	if err := row.Scan(&rm.ID, &rm.SourceAccountID, &rm.CreatedAt, &rm.UpdatedAt,
		&rm.Name, &rm.Description, &rm.Location, &rm.Floor, &rm.Building,
		&rm.Capacity, &rm.HasVideoConference, &rm.HasProjector, &rm.HasWhiteboard, &rm.HasPhone,
		&amenities, &rm.IsActive, &rm.BookingBufferMinutes, &rm.IsPublic, &allowedGroups, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(amenities, &rm.Amenities)
	json.Unmarshal(allowedGroups, &rm.AllowedGroups)
	json.Unmarshal(meta, &rm.Metadata)
	writeJSON(w, http.StatusOK, rm)
}

func (h *Handlers) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_rooms WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Events
// =============================================================================

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	q := r.URL.Query()
	conditions := []string{"source_account_id = $1"}
	params := []any{sa}
	idx := 2

	if v := q.Get("calendar_id"); v != "" {
		conditions = append(conditions, "calendar_id = $"+itoa(idx))
		params = append(params, v)
		idx++
	}
	if v := q.Get("room_id"); v != "" {
		conditions = append(conditions, "room_id = $"+itoa(idx))
		params = append(params, v)
		idx++
	}
	if v := q.Get("organizer_id"); v != "" {
		conditions = append(conditions, "organizer_id = $"+itoa(idx))
		params = append(params, v)
		idx++
	}
	if v := q.Get("status"); v != "" {
		conditions = append(conditions, "status = $"+itoa(idx))
		params = append(params, v)
		idx++
	}
	if v := q.Get("start_time"); v != "" {
		conditions = append(conditions, "start_time >= $"+itoa(idx))
		params = append(params, v)
		idx++
	}
	if v := q.Get("end_time"); v != "" {
		conditions = append(conditions, "end_time <= $"+itoa(idx))
		params = append(params, v)
	}

	where := joinConditions(conditions)
	rows, err := h.pool.Query(r.Context(),
		"SELECT id, source_account_id, created_at, updated_at, title, description, location, video_link, "+
			"start_time, end_time, all_day, timezone, is_recurring, recurrence_rule, recurrence_parent_id, "+
			"organizer_id, calendar_id, room_id, status, visibility, google_event_id, outlook_event_id, ical_uid, metadata "+
			"FROM meetings_events WHERE "+where+" ORDER BY start_time ASC", params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []Event
	for rows.Next() {
		var e Event
		var recRule, meta []byte
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.CreatedAt, &e.UpdatedAt,
			&e.Title, &e.Description, &e.Location, &e.VideoLink,
			&e.StartTime, &e.EndTime, &e.AllDay, &e.Timezone,
			&e.IsRecurring, &recRule, &e.RecurrenceParentID,
			&e.OrganizerID, &e.CalendarID, &e.RoomID,
			&e.Status, &e.Visibility, &e.GoogleEventID, &e.OutlookEventID, &e.IcalUID, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if recRule != nil {
			json.Unmarshal(recRule, &e.RecurrenceRule)
		}
		json.Unmarshal(meta, &e.Metadata)
		items = append(items, e)
	}
	if items == nil {
		items = []Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items, "total": len(items)})
}

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_events (source_account_id, title, description, location, video_link,
		 start_time, end_time, all_day, timezone, is_recurring, recurrence_rule, recurrence_parent_id,
		 organizer_id, calendar_id, room_id, status, visibility, google_event_id, outlook_event_id, ical_uid, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		 RETURNING id, source_account_id, created_at, updated_at, title, description, location, video_link,
		 start_time, end_time, all_day, timezone, is_recurring, recurrence_rule, recurrence_parent_id,
		 organizer_id, calendar_id, room_id, status, visibility, google_event_id, outlook_event_id, ical_uid, metadata`,
		sa, body["title"], body["description"], body["location"], body["video_link"],
		body["start_time"], body["end_time"], body["all_day"], body["timezone"],
		body["is_recurring"], mustJSON(body["recurrence_rule"]), body["recurrence_parent_id"],
		body["organizer_id"], body["calendar_id"], body["room_id"],
		body["status"], body["visibility"], body["google_event_id"], body["outlook_event_id"],
		body["ical_uid"], mustJSON(body["metadata"]))
	var e Event
	var recRule, meta []byte
	if err := row.Scan(&e.ID, &e.SourceAccountID, &e.CreatedAt, &e.UpdatedAt,
		&e.Title, &e.Description, &e.Location, &e.VideoLink,
		&e.StartTime, &e.EndTime, &e.AllDay, &e.Timezone,
		&e.IsRecurring, &recRule, &e.RecurrenceParentID,
		&e.OrganizerID, &e.CalendarID, &e.RoomID,
		&e.Status, &e.Visibility, &e.GoogleEventID, &e.OutlookEventID, &e.IcalUID, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if recRule != nil {
		json.Unmarshal(recRule, &e.RecurrenceRule)
	}
	json.Unmarshal(meta, &e.Metadata)
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	row := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, title, description, location, video_link,
		 start_time, end_time, all_day, timezone, is_recurring, recurrence_rule, recurrence_parent_id,
		 organizer_id, calendar_id, room_id, status, visibility, google_event_id, outlook_event_id, ical_uid, metadata
		 FROM meetings_events WHERE id=$1 AND source_account_id=$2`, id, sa)
	var e Event
	var recRule, meta []byte
	if err := row.Scan(&e.ID, &e.SourceAccountID, &e.CreatedAt, &e.UpdatedAt,
		&e.Title, &e.Description, &e.Location, &e.VideoLink,
		&e.StartTime, &e.EndTime, &e.AllDay, &e.Timezone,
		&e.IsRecurring, &recRule, &e.RecurrenceParentID,
		&e.OrganizerID, &e.CalendarID, &e.RoomID,
		&e.Status, &e.Visibility, &e.GoogleEventID, &e.OutlookEventID, &e.IcalUID, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if recRule != nil {
		json.Unmarshal(recRule, &e.RecurrenceRule)
	}
	json.Unmarshal(meta, &e.Metadata)
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`UPDATE meetings_events SET title=COALESCE($3,title), description=$4, location=$5, video_link=$6,
		 start_time=COALESCE($7,start_time), end_time=COALESCE($8,end_time),
		 all_day=COALESCE($9,all_day), timezone=COALESCE($10,timezone),
		 status=COALESCE($11,status), visibility=COALESCE($12,visibility), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, created_at, updated_at, title, description, location, video_link,
		 start_time, end_time, all_day, timezone, is_recurring, recurrence_rule, recurrence_parent_id,
		 organizer_id, calendar_id, room_id, status, visibility, google_event_id, outlook_event_id, ical_uid, metadata`,
		id, sa, body["title"], body["description"], body["location"], body["video_link"],
		body["start_time"], body["end_time"], body["all_day"], body["timezone"],
		body["status"], body["visibility"])
	var e Event
	var recRule, meta []byte
	if err := row.Scan(&e.ID, &e.SourceAccountID, &e.CreatedAt, &e.UpdatedAt,
		&e.Title, &e.Description, &e.Location, &e.VideoLink,
		&e.StartTime, &e.EndTime, &e.AllDay, &e.Timezone,
		&e.IsRecurring, &recRule, &e.RecurrenceParentID,
		&e.OrganizerID, &e.CalendarID, &e.RoomID,
		&e.Status, &e.Visibility, &e.GoogleEventID, &e.OutlookEventID, &e.IcalUID, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if recRule != nil {
		json.Unmarshal(recRule, &e.RecurrenceRule)
	}
	json.Unmarshal(meta, &e.Metadata)
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_events WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Attendees
// =============================================================================

func (h *Handlers) ListAttendees(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, event_id, user_id, external_email, external_name,
		 status, response_time, is_optional, can_modify, metadata
		 FROM meetings_attendees WHERE event_id=$1 AND source_account_id=$2 ORDER BY created_at ASC`, eventID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []Attendee
	for rows.Next() {
		var a Attendee
		var meta []byte
		if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.CreatedAt, &a.UpdatedAt,
			&a.EventID, &a.UserID, &a.ExternalEmail, &a.ExternalName,
			&a.Status, &a.ResponseTime, &a.IsOptional, &a.CanModify, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		json.Unmarshal(meta, &a.Metadata)
		items = append(items, a)
	}
	if items == nil {
		items = []Attendee{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"attendees": items, "total": len(items)})
}

func (h *Handlers) CreateAttendee(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_attendees (source_account_id, event_id, user_id, external_email, external_name,
		 status, is_optional, can_modify, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id, source_account_id, created_at, updated_at, event_id, user_id, external_email, external_name,
		 status, response_time, is_optional, can_modify, metadata`,
		sa, eventID, body["user_id"], body["external_email"], body["external_name"],
		body["status"], body["is_optional"], body["can_modify"], mustJSON(body["metadata"]))
	var a Attendee
	var meta []byte
	if err := row.Scan(&a.ID, &a.SourceAccountID, &a.CreatedAt, &a.UpdatedAt,
		&a.EventID, &a.UserID, &a.ExternalEmail, &a.ExternalName,
		&a.Status, &a.ResponseTime, &a.IsOptional, &a.CanModify, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	json.Unmarshal(meta, &a.Metadata)
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handlers) UpdateAttendeeRSVP(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`UPDATE meetings_attendees SET status=COALESCE($3,status), response_time=NOW(), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, created_at, updated_at, event_id, user_id, external_email, external_name,
		 status, response_time, is_optional, can_modify, metadata`,
		id, sa, body["status"])
	var a Attendee
	var meta []byte
	if err := row.Scan(&a.ID, &a.SourceAccountID, &a.CreatedAt, &a.UpdatedAt,
		&a.EventID, &a.UserID, &a.ExternalEmail, &a.ExternalName,
		&a.Status, &a.ResponseTime, &a.IsOptional, &a.CanModify, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(meta, &a.Metadata)
	writeJSON(w, http.StatusOK, a)
}

func (h *Handlers) DeleteAttendee(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_attendees WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Calendar Shares
// =============================================================================

func (h *Handlers) ListCalendarShares(w http.ResponseWriter, r *http.Request) {
	calendarID := chi.URLParam(r, "calendar_id")
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, calendar_id, user_id, permission
		 FROM meetings_calendar_shares WHERE calendar_id=$1 AND source_account_id=$2`, calendarID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []CalendarShare
	for rows.Next() {
		var cs CalendarShare
		if err := rows.Scan(&cs.ID, &cs.SourceAccountID, &cs.CreatedAt, &cs.CalendarID, &cs.UserID, &cs.Permission); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, cs)
	}
	if items == nil {
		items = []CalendarShare{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": items, "total": len(items)})
}

func (h *Handlers) CreateCalendarShare(w http.ResponseWriter, r *http.Request) {
	calendarID := chi.URLParam(r, "calendar_id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_calendar_shares (source_account_id, calendar_id, user_id, permission)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, source_account_id, created_at, calendar_id, user_id, permission`,
		sa, calendarID, body["user_id"], body["permission"])
	var cs CalendarShare
	if err := row.Scan(&cs.ID, &cs.SourceAccountID, &cs.CreatedAt, &cs.CalendarID, &cs.UserID, &cs.Permission); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, cs)
}

func (h *Handlers) DeleteCalendarShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_calendar_shares WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// External Calendars
// =============================================================================

func (h *Handlers) ListExternalCalendars(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, user_id, provider,
		 access_token, refresh_token, token_expires_at, provider_calendar_id, provider_user_id,
		 sync_enabled, last_sync_at, sync_direction, metadata
		 FROM meetings_external_calendars WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []ExternalCalendar
	for rows.Next() {
		var ec ExternalCalendar
		var meta []byte
		if err := rows.Scan(&ec.ID, &ec.SourceAccountID, &ec.CreatedAt, &ec.UpdatedAt,
			&ec.UserID, &ec.Provider, &ec.AccessToken, &ec.RefreshToken, &ec.TokenExpiresAt,
			&ec.ProviderCalendarID, &ec.ProviderUserID, &ec.SyncEnabled, &ec.LastSyncAt,
			&ec.SyncDirection, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		json.Unmarshal(meta, &ec.Metadata)
		items = append(items, ec)
	}
	if items == nil {
		items = []ExternalCalendar{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"external_calendars": items, "total": len(items)})
}

func (h *Handlers) CreateExternalCalendar(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_external_calendars (source_account_id, user_id, provider, access_token, refresh_token,
		 token_expires_at, provider_calendar_id, provider_user_id, sync_enabled, sync_direction, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id, source_account_id, created_at, updated_at, user_id, provider,
		 access_token, refresh_token, token_expires_at, provider_calendar_id, provider_user_id,
		 sync_enabled, last_sync_at, sync_direction, metadata`,
		sa, body["user_id"], body["provider"], body["access_token"], body["refresh_token"],
		body["token_expires_at"], body["provider_calendar_id"], body["provider_user_id"],
		body["sync_enabled"], body["sync_direction"], mustJSON(body["metadata"]))
	var ec ExternalCalendar
	var meta []byte
	if err := row.Scan(&ec.ID, &ec.SourceAccountID, &ec.CreatedAt, &ec.UpdatedAt,
		&ec.UserID, &ec.Provider, &ec.AccessToken, &ec.RefreshToken, &ec.TokenExpiresAt,
		&ec.ProviderCalendarID, &ec.ProviderUserID, &ec.SyncEnabled, &ec.LastSyncAt,
		&ec.SyncDirection, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	json.Unmarshal(meta, &ec.Metadata)
	writeJSON(w, http.StatusCreated, ec)
}

func (h *Handlers) DeleteExternalCalendar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_external_calendars WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Templates
// =============================================================================

func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, default_title,
		 default_description, default_duration_minutes, default_location, default_video_link,
		 default_attendees, created_by, is_public, metadata
		 FROM meetings_templates WHERE source_account_id=$1 ORDER BY name`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []Template
	for rows.Next() {
		var t Template
		var defaultAttendees, meta []byte
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.CreatedAt, &t.UpdatedAt,
			&t.Name, &t.Description, &t.DefaultTitle, &t.DefaultDescription,
			&t.DefaultDurationMinutes, &t.DefaultLocation, &t.DefaultVideoLink,
			&defaultAttendees, &t.CreatedBy, &t.IsPublic, &meta); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		json.Unmarshal(defaultAttendees, &t.DefaultAttendees)
		json.Unmarshal(meta, &t.Metadata)
		items = append(items, t)
	}
	if items == nil {
		items = []Template{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": items, "total": len(items)})
}

func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_templates (source_account_id, name, description, default_title, default_description,
		 default_duration_minutes, default_location, default_video_link, default_attendees, created_by, is_public, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING id, source_account_id, created_at, updated_at, name, description, default_title,
		 default_description, default_duration_minutes, default_location, default_video_link,
		 default_attendees, created_by, is_public, metadata`,
		sa, body["name"], body["description"], body["default_title"], body["default_description"],
		body["default_duration_minutes"], body["default_location"], body["default_video_link"],
		mustJSON(body["default_attendees"]), body["created_by"], body["is_public"], mustJSON(body["metadata"]))
	var t Template
	var defaultAttendees, meta []byte
	if err := row.Scan(&t.ID, &t.SourceAccountID, &t.CreatedAt, &t.UpdatedAt,
		&t.Name, &t.Description, &t.DefaultTitle, &t.DefaultDescription,
		&t.DefaultDurationMinutes, &t.DefaultLocation, &t.DefaultVideoLink,
		&defaultAttendees, &t.CreatedBy, &t.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	json.Unmarshal(defaultAttendees, &t.DefaultAttendees)
	json.Unmarshal(meta, &t.Metadata)
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	row := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, created_at, updated_at, name, description, default_title,
		 default_description, default_duration_minutes, default_location, default_video_link,
		 default_attendees, created_by, is_public, metadata
		 FROM meetings_templates WHERE id=$1 AND source_account_id=$2`, id, sa)
	var t Template
	var defaultAttendees, meta []byte
	if err := row.Scan(&t.ID, &t.SourceAccountID, &t.CreatedAt, &t.UpdatedAt,
		&t.Name, &t.Description, &t.DefaultTitle, &t.DefaultDescription,
		&t.DefaultDurationMinutes, &t.DefaultLocation, &t.DefaultVideoLink,
		&defaultAttendees, &t.CreatedBy, &t.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(defaultAttendees, &t.DefaultAttendees)
	json.Unmarshal(meta, &t.Metadata)
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`UPDATE meetings_templates SET name=COALESCE($3,name), description=$4, default_title=$5,
		 default_description=$6, default_duration_minutes=COALESCE($7,default_duration_minutes),
		 default_location=$8, default_video_link=$9, is_public=COALESCE($10,is_public), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2
		 RETURNING id, source_account_id, created_at, updated_at, name, description, default_title,
		 default_description, default_duration_minutes, default_location, default_video_link,
		 default_attendees, created_by, is_public, metadata`,
		id, sa, body["name"], body["description"], body["default_title"], body["default_description"],
		body["default_duration_minutes"], body["default_location"], body["default_video_link"], body["is_public"])
	var t Template
	var defaultAttendees, meta []byte
	if err := row.Scan(&t.ID, &t.SourceAccountID, &t.CreatedAt, &t.UpdatedAt,
		&t.Name, &t.Description, &t.DefaultTitle, &t.DefaultDescription,
		&t.DefaultDurationMinutes, &t.DefaultLocation, &t.DefaultVideoLink,
		&defaultAttendees, &t.CreatedBy, &t.IsPublic, &meta); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	json.Unmarshal(defaultAttendees, &t.DefaultAttendees)
	json.Unmarshal(meta, &t.Metadata)
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_templates WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Reminders
// =============================================================================

func (h *Handlers) ListReminders(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	sa := h.sa(r)
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, created_at, event_id, user_id, remind_at, minutes_before, method, sent, sent_at
		 FROM meetings_reminders WHERE event_id=$1 AND source_account_id=$2 ORDER BY remind_at`, eventID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var items []Reminder
	for rows.Next() {
		var rem Reminder
		if err := rows.Scan(&rem.ID, &rem.SourceAccountID, &rem.CreatedAt,
			&rem.EventID, &rem.UserID, &rem.RemindAt, &rem.MinutesBefore,
			&rem.Method, &rem.Sent, &rem.SentAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, rem)
	}
	if items == nil {
		items = []Reminder{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": items, "total": len(items)})
}

func (h *Handlers) CreateReminder(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "event_id")
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := h.pool.QueryRow(r.Context(),
		`INSERT INTO meetings_reminders (source_account_id, event_id, user_id, remind_at, minutes_before, method)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, source_account_id, created_at, event_id, user_id, remind_at, minutes_before, method, sent, sent_at`,
		sa, eventID, body["user_id"], body["remind_at"], body["minutes_before"], body["method"])
	var rem Reminder
	if err := row.Scan(&rem.ID, &rem.SourceAccountID, &rem.CreatedAt,
		&rem.EventID, &rem.UserID, &rem.RemindAt, &rem.MinutesBefore,
		&rem.Method, &rem.Sent, &rem.SentAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rem)
}

func (h *Handlers) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa := h.sa(r)
	h.pool.Exec(r.Context(), `DELETE FROM meetings_reminders WHERE id=$1 AND source_account_id=$2`, id, sa)
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Helpers
// =============================================================================

func mustJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

func itoa(i int) string {
	return string(rune('0'+i)) // simple single-digit; safe for param counts we use
}

func joinConditions(conds []string) string {
	result := ""
	for i, c := range conds {
		if i > 0 {
			result += " AND "
		}
		result += c
	}
	return result
}
