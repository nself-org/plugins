package main

// handlers_components.go — /v1/status-pages/{id}/components: explicit
// curation of which monitors a status page shows publicly.
//
// Purpose: replace the host-shape heuristic with tenant intent. When a page
//   has component rows, the public renderer (handlers_statuspublic.go) shows
//   EXACTLY the rows with public=true; a new component defaults to
//   public=false (FAIL-CLOSED — nothing becomes visible by accident).
// Routes:  GET    /v1/status-pages/{id}/components
//          POST   /v1/status-pages/{id}/components  {"monitor_id","name"?,"public"?}
//          DELETE /v1/status-pages/{id}/components/{component_id}
// Outputs: {"components":[...]}, {"component":{...}}.
// Constraints — P0 tenancy: the page must belong to the verified tenant
//   (404 otherwise, indistinguishable from missing); component rows carry
//   tenant_id and every mutation re-checks it.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// componentDTO is the contract StatusPageComponent shape.
type componentDTO struct {
	ID        string `json:"id"`
	MonitorID string `json:"monitor_id"`
	Name      string `json:"name,omitempty"`
	Public    bool   `json:"public"`
	Position  int    `json:"position"`
	CreatedAt string `json:"created_at"`
}

// tenantOwnsPage verifies the status page belongs to the tenant. Writes the
// generic 404 and returns false when it does not (or on invalid id).
func (g *gateway) tenantOwnsPage(w http.ResponseWriter, r *http.Request, pageID, tenantID string) bool {
	if uuid.Validate(pageID) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "status page not found")
		return false
	}
	var owned string
	err := g.db.QueryRowContext(r.Context(),
		`SELECT id::text FROM np_saas_status_pages WHERE id = $1 AND tenant_id = $2`,
		pageID, tenantID).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "status page not found")
		return false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "status page lookup failed")
		return false
	}
	return true
}

// handleListComponents — GET /v1/status-pages/{id}/components.
func (g *gateway) handleListComponents(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	pageID := chi.URLParam(r, "id")
	if !g.tenantOwnsPage(w, r, pageID, tenantID) {
		return
	}

	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, monitor_id, name, public, position, created_at
		FROM np_saas_status_page_components
		WHERE status_page_id = $1 AND tenant_id = $2
		ORDER BY position ASC, created_at ASC`, pageID, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "component lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	components := make([]componentDTO, 0)
	for rows.Next() {
		var c componentDTO
		var createdAt time.Time
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.Name, &c.Public, &c.Position, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "component scan failed")
			return
		}
		c.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		components = append(components, c)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "component iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"components": components})
}

// handleCreateComponent — POST /v1/status-pages/{id}/components.
// public defaults to FALSE (fail-closed).
func (g *gateway) handleCreateComponent(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	pageID := chi.URLParam(r, "id")
	if !g.tenantOwnsPage(w, r, pageID, tenantID) {
		return
	}

	var req struct {
		MonitorID string `json:"monitor_id"`
		Name      string `json:"name"`
		Public    bool   `json:"public"` // zero value false = fail-closed
		Position  int    `json:"position"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.MonitorID = strings.TrimSpace(req.MonitorID)
	if req.MonitorID == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "monitor_id is required")
		return
	}

	var c componentDTO
	var createdAt time.Time
	err := g.db.QueryRowContext(r.Context(), `
		INSERT INTO np_saas_status_page_components
			(tenant_id, status_page_id, monitor_id, name, public, position)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, monitor_id, name, public, position, created_at`,
		tenantID, pageID, req.MonitorID, strings.TrimSpace(req.Name), req.Public, req.Position).
		Scan(&c.ID, &c.MonitorID, &c.Name, &c.Public, &c.Position, &createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "already_added",
				"this monitor is already a component of the page")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "component create failed")
		return
	}
	c.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, map[string]any{"component": c})
}

// handleDeleteComponent — DELETE /v1/status-pages/{id}/components/{component_id}.
func (g *gateway) handleDeleteComponent(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	pageID := chi.URLParam(r, "id")
	componentID := chi.URLParam(r, "component_id")
	if !g.tenantOwnsPage(w, r, pageID, tenantID) {
		return
	}
	if uuid.Validate(componentID) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "component not found")
		return
	}
	res, err := g.db.ExecContext(r.Context(), `
		DELETE FROM np_saas_status_page_components
		WHERE id = $1 AND status_page_id = $2 AND tenant_id = $3`,
		componentID, pageID, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "component delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "component not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
