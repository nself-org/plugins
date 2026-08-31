package main

// handlers_subscribers.go — /v1/subscribers: status-page email subscribers.
//
// Purpose: the addresses that receive public status-page notifications for a
//   tenant (all pages, or one page when status_page_id is set). Rows live in
//   the gateway-owned np_saas_subscribers table; the notifier consumes them.
// Routes:  GET/POST /v1/subscribers · DELETE /v1/subscribers/{id}
// Outputs: {"subscribers":[...]}, {"subscriber":{...}}.
// Constraints — P0 tenancy: rows are scoped by the verified tenant;
//   a status_page_id in the body must belong to the tenant (404 otherwise —
//   indistinguishable from a missing page); cross-tenant subscriber ids 404.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// subscriberDTO is the contract Subscriber shape.
type subscriberDTO struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	StatusPageID string `json:"status_page_id,omitempty"` // empty = all pages
	CreatedAt    string `json:"created_at"`
}

// handleListSubscribers — GET /v1/subscribers.
func (g *gateway) handleListSubscribers(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, email, status_page_id::text, created_at
		FROM np_saas_subscribers
		WHERE tenant_id = $1
		ORDER BY created_at ASC`, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "subscriber lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	subs := make([]subscriberDTO, 0)
	for rows.Next() {
		var s subscriberDTO
		var pageID sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&s.ID, &s.Email, &pageID, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "subscriber scan failed")
			return
		}
		s.StatusPageID = pageID.String
		s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		subs = append(subs, s)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "subscriber iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscribers": subs})
}

// handleCreateSubscriber — POST /v1/subscribers {"email","status_page_id"?}.
func (g *gateway) handleCreateSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	var req struct {
		Email        string `json:"email"`
		StatusPageID string `json:"status_page_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "a valid email is required")
		return
	}

	// A page id, when given, must be the TENANT'S page (404 otherwise).
	var pageID any // nil → all-pages subscription
	if req.StatusPageID != "" {
		if uuid.Validate(req.StatusPageID) != nil {
			writeErr(w, http.StatusNotFound, "not_found", "status page not found")
			return
		}
		var owned string
		err := g.db.QueryRowContext(r.Context(),
			`SELECT id::text FROM np_saas_status_pages WHERE id = $1 AND tenant_id = $2`,
			req.StatusPageID, tenantID).Scan(&owned)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "status page not found")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "status page lookup failed")
			return
		}
		pageID = req.StatusPageID
	}

	var s subscriberDTO
	var gotPageID sql.NullString
	var createdAt time.Time
	err := g.db.QueryRowContext(r.Context(), `
		INSERT INTO np_saas_subscribers (tenant_id, status_page_id, email)
		VALUES ($1, $2, $3)
		RETURNING id::text, email, status_page_id::text, created_at`,
		tenantID, pageID, req.Email).
		Scan(&s.ID, &s.Email, &gotPageID, &createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "already_subscribed",
				"this email is already subscribed")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "subscriber create failed")
		return
	}
	s.StatusPageID = gotPageID.String
	s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, map[string]any{"subscriber": s})
}

// handleDeleteSubscriber — DELETE /v1/subscribers/{id}.
func (g *gateway) handleDeleteSubscriber(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "subscriber not found")
		return
	}
	res, err := g.db.ExecContext(r.Context(),
		`DELETE FROM np_saas_subscribers WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "subscriber delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "subscriber not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
