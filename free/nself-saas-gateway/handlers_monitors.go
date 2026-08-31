package main

// handlers_monitors.go — /v1/monitors → nself-uptime-monitor (/api/v1/targets).
//
// Purpose: map the unified Monitor contract (cli/internal/sentryapi) onto the
//   uptime plugin's target API. The plugin enforces tenant scoping, the
//   monitors count quota (402), and the check-frequency floor itself; the
//   gateway only translates shapes.
// Outputs: {"monitors":[...]}, {"monitor":{...}}, 204 on delete.
// Constraints: Monitor.Status is up|down|paused|pending — derived from the
//   target's enabled flag plus its most recent probe result.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
)

// upstreamTarget mirrors nself-uptime-monitor's targetRow (fields we map).
type upstreamTarget struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Protocol     string    `json:"protocol"`
	IntervalSecs int       `json:"interval_secs"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// upstreamResult mirrors the plugin's resultRow (fields we map).
type upstreamResult struct {
	TargetID string `json:"target_id"`
	Status   string `json:"status"`
}

// monitorDTO is the contract Monitor shape.
type monitorDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Kind            string `json:"kind"`
	IntervalSeconds int    `json:"interval_seconds"`
	Status          string `json:"status"`
	Paused          bool   `json:"paused"`
	CreatedAt       string `json:"created_at"`
}

// monitorStatus derives the contract status for a target.
func monitorStatus(t upstreamTarget, lastResult string) string {
	if !t.Enabled {
		return "paused"
	}
	switch lastResult {
	case "up", "down":
		return lastResult
	}
	return "pending"
}

func toMonitorDTO(t upstreamTarget, lastResult string) monitorDTO {
	return monitorDTO{
		ID:              t.ID,
		Name:            t.Name,
		URL:             t.URL,
		Kind:            t.Protocol,
		IntervalSeconds: t.IntervalSecs,
		Status:          monitorStatus(t, lastResult),
		Paused:          !t.Enabled,
		CreatedAt:       t.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// lastResults fetches recent probe results and reduces them to the newest
// status per target (results arrive newest-first). Best-effort: an error
// yields an empty map and monitors show "pending".
func (g *gateway) lastResults(r *http.Request, targetID string) map[string]string {
	path := "/api/v1/results?limit=500"
	if targetID != "" {
		path = "/api/v1/results?limit=1&target_id=" + url.QueryEscape(targetID)
	}
	status, body, err := g.callUpstream(r.Context(), r, http.MethodGet, g.cfg.UptimeURL, path, nil)
	if err != nil || !upstreamOK(status) {
		return map[string]string{}
	}
	var env struct {
		Results []upstreamResult `json:"results"`
	}
	if json.Unmarshal(body, &env) != nil {
		return map[string]string{}
	}
	last := make(map[string]string, len(env.Results))
	for _, res := range env.Results {
		if _, seen := last[res.TargetID]; !seen {
			last[res.TargetID] = res.Status
		}
	}
	return last
}

// handleListMonitors — GET /v1/monitors.
func (g *gateway) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	status, body, err := g.callUpstream(r.Context(), r, http.MethodGet, g.cfg.UptimeURL, "/api/v1/targets/", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var env struct {
		Targets []upstreamTarget `json:"targets"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	last := g.lastResults(r, "")
	monitors := make([]monitorDTO, 0, len(env.Targets))
	for _, t := range env.Targets {
		monitors = append(monitors, toMonitorDTO(t, last[t.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"monitors": monitors})
}

// fetchMonitor GETs one target and maps it (with last probe status).
func (g *gateway) fetchMonitor(w http.ResponseWriter, r *http.Request, id string, respStatus int) {
	status, body, err := g.callUpstream(r.Context(), r, http.MethodGet, g.cfg.UptimeURL, "/api/v1/targets/"+url.PathEscape(id), nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var t upstreamTarget
	if err := json.Unmarshal(body, &t); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	last := g.lastResults(r, id)
	writeJSON(w, respStatus, map[string]any{"monitor": toMonitorDTO(t, last[id])})
}

// handleCreateMonitor — POST /v1/monitors.
func (g *gateway) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		URL             string `json:"url"`
		Kind            string `json:"kind"`
		IntervalSeconds int    `json:"interval_seconds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	upstreamReq := map[string]any{
		"name":          req.Name,
		"url":           req.URL,
		"protocol":      req.Kind,
		"interval_secs": req.IntervalSeconds,
	}
	status, body, err := g.callUpstream(r.Context(), r, http.MethodPost, g.cfg.UptimeURL, "/api/v1/targets/", upstreamReq)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	// The plugin's create response is partial ({id,name,url,protocol}); fetch
	// the full row so the contract envelope is complete.
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream create response")
		return
	}
	g.fetchMonitor(w, r, created.ID, http.StatusCreated)
}

// handleDeleteMonitor — DELETE /v1/monitors/{id}.
func (g *gateway) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, body, err := g.callUpstream(r.Context(), r, http.MethodDelete, g.cfg.UptimeURL, "/api/v1/targets/"+url.PathEscape(id), nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateMonitor — PATCH /v1/monitors/{id}.
// Partial edit of a monitor's name/url/kind/interval (proxied to the uptime
// plugin's PUT /api/v1/targets/{id}, which enforces tenant scoping + the SSRF
// guard on URL change + the tier interval floor) and/or paused state (mapped
// to the plugin's pause/resume endpoints). Cross-tenant ids 404 upstream.
func (g *gateway) handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name            *string `json:"name"`
		URL             *string `json:"url"`
		Kind            *string `json:"kind"`
		IntervalSeconds *int    `json:"interval_seconds"`
		Paused          *bool   `json:"paused"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}

	// Field edit → upstream PUT (partial: only set fields are forwarded).
	if req.Name != nil || req.URL != nil || req.Kind != nil || req.IntervalSeconds != nil {
		upstreamReq := map[string]any{}
		if req.Name != nil {
			upstreamReq["name"] = *req.Name
		}
		if req.URL != nil {
			upstreamReq["url"] = *req.URL
		}
		if req.Kind != nil {
			upstreamReq["protocol"] = *req.Kind
		}
		if req.IntervalSeconds != nil {
			upstreamReq["interval_secs"] = *req.IntervalSeconds
		}
		status, body, err := g.callUpstream(r.Context(), r, http.MethodPut, g.cfg.UptimeURL,
			"/api/v1/targets/"+url.PathEscape(id), upstreamReq)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
			return
		}
		if !upstreamOK(status) {
			relayUpstreamError(w, status, body)
			return
		}
	}

	// Paused transition → pause/resume endpoint.
	if req.Paused != nil {
		action := "resume"
		if *req.Paused {
			action = "pause"
		}
		status, body, err := g.callUpstream(r.Context(), r, http.MethodPost, g.cfg.UptimeURL,
			"/api/v1/targets/"+url.PathEscape(id)+"/"+action, nil)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
			return
		}
		if !upstreamOK(status) {
			relayUpstreamError(w, status, body)
			return
		}
	}

	g.fetchMonitor(w, r, id, http.StatusOK)
}

// handlePauseResumeMonitor — POST /v1/monitors/{id}/pause|resume.
func (g *gateway) handlePauseResumeMonitor(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		status, body, err := g.callUpstream(r.Context(), r, http.MethodPost, g.cfg.UptimeURL,
			"/api/v1/targets/"+url.PathEscape(id)+"/"+action, nil)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
			return
		}
		if !upstreamOK(status) {
			relayUpstreamError(w, status, body)
			return
		}
		g.fetchMonitor(w, r, id, http.StatusOK)
	}
}
