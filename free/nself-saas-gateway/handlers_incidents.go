package main

// handlers_incidents.go — /v1/incidents → nself-incident-mgmt (/incidents).
//
// Purpose: map the contract Incident (open|acknowledged|resolved) onto the
//   incident plugin's state machine (open|acknowledged|mitigating|resolved).
//   "mitigating" is presented as "acknowledged" in the v1 contract (an
//   engineer is actively working — the closest contract state).
// Outputs: {"incidents":[...]}, {"incident":{...}}.
// Constraints: the plugin has no monitor linkage column yet, so monitor_id
//   is empty until the uptime→incident bridge lands (tracked in the plan).

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

// upstreamIncident mirrors nself-incident-mgmt's store.Incident (mapped fields).
type upstreamIncident struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Severity       string     `json:"severity"`
	State          string     `json:"state"`
	CreatedAt      time.Time  `json:"created_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

// incidentDTO is the contract Incident shape.
type incidentDTO struct {
	ID             string `json:"id"`
	MonitorID      string `json:"monitor_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Severity       string `json:"severity"`
	StartedAt      string `json:"started_at"`
	AcknowledgedAt string `json:"acknowledged_at,omitempty"`
	ResolvedAt     string `json:"resolved_at,omitempty"`
}

// contractIncidentStatus maps plugin states onto the v1 contract enum.
func contractIncidentStatus(state string) string {
	if state == "mitigating" {
		return "acknowledged"
	}
	return state
}

// upstreamStateFilter maps a contract ?status= onto the plugin ?state=.
func upstreamStateFilter(status string) string {
	// "acknowledged" folds mitigating in on the response side only; the
	// upstream filter passes through 1:1 for the three contract values.
	return status
}

func toIncidentDTO(in upstreamIncident) incidentDTO {
	dto := incidentDTO{
		ID:        in.ID,
		Title:     in.Title,
		Status:    contractIncidentStatus(in.State),
		Severity:  in.Severity,
		StartedAt: in.CreatedAt.UTC().Format(time.RFC3339),
	}
	if in.AcknowledgedAt != nil {
		dto.AcknowledgedAt = in.AcknowledgedAt.UTC().Format(time.RFC3339)
	}
	if in.ResolvedAt != nil {
		dto.ResolvedAt = in.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

// handleListIncidents — GET /v1/incidents?status=.
func (g *gateway) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	path := "/incidents/"
	if s := r.URL.Query().Get("status"); s != "" {
		path += "?state=" + url.QueryEscape(upstreamStateFilter(s))
	}
	status, body, err := g.callUpstream(r.Context(), r, http.MethodGet, g.cfg.IncidentURL, path, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var env struct {
		Incidents []upstreamIncident `json:"incidents"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	incidents := make([]incidentDTO, 0, len(env.Incidents))
	for _, in := range env.Incidents {
		incidents = append(incidents, toIncidentDTO(in))
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents})
}

// timelineEntryDTO is one incident update in the contract shape.
type timelineEntryDTO struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Actor      string `json:"actor,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

// upstreamTimelineEntry mirrors nself-incident-mgmt's store.TimelineEntry.
type upstreamTimelineEntry struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Message    string    `json:"message"`
	Actor      *string   `json:"actor,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// upstreamIncidentDetail mirrors store.IncidentDetail (incident + timeline).
type upstreamIncidentDetail struct {
	upstreamIncident
	Description string                  `json:"description"`
	Timeline    []upstreamTimelineEntry `json:"timeline"`
}

// validSeverity is the plugin's severity enum.
func validSeverity(s string) bool {
	switch s {
	case "", "critical", "high", "medium", "low", "info":
		return true
	}
	return false
}

// contractToUpstreamState maps a contract status onto the plugin state enum.
// "acknowledged" is a 1:1 state; "mitigating" only ever appears upstream.
func contractToUpstreamState(status string) (string, bool) {
	switch status {
	case "open", "acknowledged", "resolved":
		return status, true
	}
	return "", false
}

// handleCreateIncident — POST /v1/incidents {"title","description"?,"severity"?}.
func (g *gateway) handleCreateIncident(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "title is required")
		return
	}
	if !validSeverity(req.Severity) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"severity must be critical, high, medium, low, or info")
		return
	}
	status, body, err := g.callUpstream(r.Context(), r, http.MethodPost, g.cfg.IncidentURL,
		"/incidents/", map[string]any{
			"title":       req.Title,
			"description": req.Description,
			"severity":    req.Severity,
		})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var in upstreamIncident
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"incident": toIncidentDTO(in)})
}

// handleGetIncident — GET /v1/incidents/{id}: incident + timeline updates.
// Tenant scoping is enforced by the incident plugin against the service-JWT
// tenant (proxy.go) — another tenant's incident id is upstream-404.
func (g *gateway) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, body, err := g.callUpstream(r.Context(), r, http.MethodGet, g.cfg.IncidentURL,
		"/incidents/"+url.PathEscape(id)+"/", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var detail upstreamIncidentDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	updates := make([]timelineEntryDTO, 0, len(detail.Timeline))
	for _, e := range detail.Timeline {
		dto := timelineEntryDTO{
			ID:         e.ID,
			Kind:       e.Kind,
			Message:    e.Message,
			OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339),
		}
		if e.Actor != nil {
			dto.Actor = *e.Actor
		}
		updates = append(updates, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incident":    toIncidentDTO(detail.upstreamIncident),
		"description": detail.Description,
		"updates":     updates,
	})
}

// handleUpdateIncident — PATCH /v1/incidents/{id}
// {"title"?,"description"?,"severity"?,"status"?}.
func (g *gateway) handleUpdateIncident(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Severity    *string `json:"severity"`
		Status      *string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	if req.Severity != nil && !validSeverity(*req.Severity) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"severity must be critical, high, medium, low, or info")
		return
	}
	body := map[string]any{}
	if req.Title != nil {
		body["title"] = *req.Title
	}
	if req.Description != nil {
		body["description"] = *req.Description
	}
	if req.Severity != nil {
		body["severity"] = *req.Severity
	}
	if req.Status != nil {
		state, ok := contractToUpstreamState(*req.Status)
		if !ok {
			writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
				"status must be open, acknowledged, or resolved")
			return
		}
		body["state"] = state
	}
	if len(body) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "no fields to update")
		return
	}
	status, respBody, err := g.callUpstream(r.Context(), r, http.MethodPatch, g.cfg.IncidentURL,
		"/incidents/"+url.PathEscape(id)+"/", body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, respBody)
		return
	}
	var in upstreamIncident
	if err := json.Unmarshal(respBody, &in); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": toIncidentDTO(in)})
}

// handleIncidentAction — POST /v1/incidents/{id}/ack|resolve.
// action is the upstream transition path segment.
func (g *gateway) handleIncidentAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		status, body, err := g.callUpstream(r.Context(), r, http.MethodPost, g.cfg.IncidentURL,
			"/incidents/"+url.PathEscape(id)+"/"+action, nil)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
			return
		}
		if !upstreamOK(status) {
			relayUpstreamError(w, status, body)
			return
		}
		var in upstreamIncident
		if err := json.Unmarshal(body, &in); err != nil {
			writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"incident": toIncidentDTO(in)})
	}
}
