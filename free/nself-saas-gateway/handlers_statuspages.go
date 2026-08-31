package main

// handlers_statuspages.go — /v1/status-pages → np_saas_status_pages registry.
//
// Purpose: status pages are per-tenant INSTANCES of nself-status-page (one
//   process = one page, scoped via STATUS_PAGE_TENANT_ID), so there is no
//   multi-page plugin API to proxy. The gateway owns the tenant's page
//   REGISTRY: create enforces the StatusPages tier quota (402) and records
//   the page; the W2 box orchestration provisions the actual instance from
//   this table. Public URL: {StatusPageBaseURL}/{slug} (PRD §4).
// Outputs: {"status_pages":[...]}, 201 {"status_page":{...}}.
// Constraints: slugs are globally unique (they are public URL paths).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// statusPageDTO is the contract StatusPage shape.
type statusPageDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	URL       string `json:"url"`
	Public    bool   `json:"public"`
	CreatedAt string `json:"created_at"`
}

// slugPattern: lowercase DNS-ish slugs, 3-63 chars.
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,61}[a-z0-9])$`)

func (g *gateway) pageURL(slug string) string {
	return strings.TrimRight(g.cfg.StatusPageBaseURL, "/") + "/" + slug
}

// handleListStatusPages — GET /v1/status-pages.
func (g *gateway) handleListStatusPages(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, name, slug, public, created_at
		FROM np_saas_status_pages
		WHERE tenant_id = $1
		ORDER BY created_at ASC`, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "status page lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	pages := make([]statusPageDTO, 0)
	for rows.Next() {
		var p statusPageDTO
		var createdAt time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Public, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "status page scan failed")
			return
		}
		p.URL = g.pageURL(p.Slug)
		p.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		pages = append(pages, p)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "status page iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status_pages": pages})
}

// handleCreateStatusPage — POST /v1/status-pages (quota-gated).
func (g *gateway) handleCreateStatusPage(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if req.Name == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "name is required")
		return
	}
	if !slugPattern.MatchString(req.Slug) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"slug must be 3-63 lowercase letters, digits, or hyphens")
		return
	}

	// StatusPages tier quota (PRD §3): free 1 / bundle 3 / plus 10.
	limits, tier, found, err := saas.EffectiveLimits(r.Context(), g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
		return
	}
	if found {
		qErr, err := saas.CheckResourceCount(r.Context(), g.db, tenantID,
			"np_saas_status_pages", limits.StatusPages, tier, "status pages")
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
			return
		}
		if qErr != nil {
			saas.WriteQuotaExceeded(w, qErr)
			return
		}
	}

	var p statusPageDTO
	var createdAt time.Time
	err = g.db.QueryRowContext(r.Context(), `
		INSERT INTO np_saas_status_pages (tenant_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id::text, name, slug, public, created_at`,
		tenantID, req.Name, req.Slug,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.Public, &createdAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
			writeErr(w, http.StatusConflict, "slug_taken", "that slug is already in use")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "status page create failed")
		return
	}
	p.URL = g.pageURL(p.Slug)
	p.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, map[string]any{"status_page": p})
}

// handleUpdateStatusPage — PATCH /v1/status-pages/{id} (tenant-scoped).
// Partial edit of a page's name/slug/public. The UPDATE is scoped to the
// caller's tenant, so another tenant's id matches zero rows → 404 (never
// acts). The SPA view model calls the name field "title" and public
// "published"; both aliases are accepted. Component health lives on the page
// INSTANCE (not this registry row), so a {components:[...]} body is a no-op
// that returns the current row rather than erroring.
func (g *gateway) handleUpdateStatusPage(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "status page id must be a UUID")
		return
	}

	var req struct {
		Name      *string `json:"name"`
		Title     *string `json:"title"`
		Slug      *string `json:"slug"`
		Public    *bool   `json:"public"`
		Published *bool   `json:"published"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}

	// Normalize SPA aliases (title→name, published→public).
	if req.Name == nil {
		req.Name = req.Title
	}
	if req.Public == nil {
		req.Public = req.Published
	}

	var nameArg sql.NullString
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "name cannot be empty")
			return
		}
		nameArg = sql.NullString{String: n, Valid: true}
	}
	var slugArg sql.NullString
	if req.Slug != nil {
		s := strings.ToLower(strings.TrimSpace(*req.Slug))
		if !slugPattern.MatchString(s) {
			writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
				"slug must be 3-63 lowercase letters, digits, or hyphens")
			return
		}
		slugArg = sql.NullString{String: s, Valid: true}
	}
	var publicArg sql.NullBool
	if req.Public != nil {
		publicArg = sql.NullBool{Bool: *req.Public, Valid: true}
	}

	var p statusPageDTO
	var createdAt time.Time
	err := g.db.QueryRowContext(r.Context(), `
		UPDATE np_saas_status_pages
		SET name   = COALESCE($3, name),
		    slug   = COALESCE($4, slug),
		    public = COALESCE($5, public)
		WHERE id = $1 AND tenant_id = $2
		RETURNING id::text, name, slug, public, created_at`,
		id, tenantID, nameArg, slugArg, publicArg,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.Public, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Not this tenant's page (or gone) — never leak existence.
		writeErr(w, http.StatusNotFound, "not_found", "status page not found")
		return
	}
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
			writeErr(w, http.StatusConflict, "slug_taken", "that slug is already in use")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "status page update failed")
		return
	}
	p.URL = g.pageURL(p.Slug)
	p.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, map[string]any{"status_page": p})
}

// handleDeleteStatusPage — DELETE /v1/status-pages/{id} (tenant-scoped).
// Deletes the caller's own page; another tenant's id matches zero rows → 404
// (never acts, never leaks existence). The SPA's remove button calls this.
func (g *gateway) handleDeleteStatusPage(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "status page id must be a UUID")
		return
	}
	res, err := g.db.ExecContext(r.Context(),
		`DELETE FROM np_saas_status_pages WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "status page delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "status page not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// errNoDB guards DB-backed handlers when DATABASE_URL is absent.
var errNoDB = errors.New("gateway database not configured")

// requireDB wraps DB-backed handlers with a 503 when no DB is attached.
func (g *gateway) requireDB(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.db == nil {
			writeErr(w, http.StatusServiceUnavailable, "db_unavailable", errNoDB.Error())
			return
		}
		next(w, r)
	}
}
