package main

// handlers_overview.go — GET /v1/overview: the dashboard landing summary.
//
// Purpose: one call the SPA makes on /app/overview — monitor up/down counts,
//   open (unresolved) incident count, recent uptime %, and the monthly
//   error/RUM ingest counters. Tenant-scoped via the verified credential
//   (never a client header — see spoof_test.go).
// Outputs: {"overview":{"monitors":{...},"incidents":{"open":N},
//   "uptime_percent_24h":F|null,"errors":{...},"rum":{...}}} (snake_case).
// Constraints: upstream reads are best-effort where cosmetic (uptime %),
//   but monitor/incident counts propagate upstream failures as 502 so the
//   dashboard never silently renders zeros for live tenants.

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// overviewMonitors is the monitor-count block of the overview.
type overviewMonitors struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Paused  int `json:"paused"`
	Pending int `json:"pending"`
}

// handleOverview — GET /v1/overview.
func (g *gateway) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Monitors: targets + newest probe status per target (same derivation as
	// /v1/monitors so the two screens can never disagree).
	status, body, err := g.callUpstream(ctx, r, http.MethodGet, g.cfg.UptimeURL, "/api/v1/targets/", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var targetsEnv struct {
		Targets []upstreamTarget `json:"targets"`
	}
	if err := json.Unmarshal(body, &targetsEnv); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	last := g.lastResults(r, "")
	var mon overviewMonitors
	mon.Total = len(targetsEnv.Targets)
	for _, t := range targetsEnv.Targets {
		switch monitorStatus(t, last[t.ID]) {
		case "up":
			mon.Up++
		case "down":
			mon.Down++
		case "paused":
			mon.Paused++
		default:
			mon.Pending++
		}
	}

	// Uptime %: share of "up" among the recent probe results (the same
	// 500-result window lastResults uses). Null when no probes have run yet.
	var uptimePct *float64
	if upStatus, upBody, upErr := g.callUpstream(ctx, r, http.MethodGet, g.cfg.UptimeURL, "/api/v1/results?limit=500", nil); upErr == nil && upstreamOK(upStatus) {
		var resEnv struct {
			Results []upstreamResult `json:"results"`
		}
		if json.Unmarshal(upBody, &resEnv) == nil {
			var up, down int
			for _, res := range resEnv.Results {
				switch res.Status {
				case "up":
					up++
				case "down":
					down++
				}
			}
			if up+down > 0 {
				pct := math.Round(float64(up)/float64(up+down)*10000) / 100
				uptimePct = &pct
			}
		}
	}

	// Open incidents: everything not yet resolved (open|acknowledged|mitigating).
	openIncidents := 0
	incStatus, incBody, incErr := g.callUpstream(ctx, r, http.MethodGet, g.cfg.IncidentURL, "/incidents/", nil)
	if incErr != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", incErr.Error())
		return
	}
	if !upstreamOK(incStatus) {
		relayUpstreamError(w, incStatus, incBody)
		return
	}
	var incEnv struct {
		Incidents []upstreamIncident `json:"incidents"`
	}
	if err := json.Unmarshal(incBody, &incEnv); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid upstream response")
		return
	}
	for _, in := range incEnv.Incidents {
		if in.State != "resolved" {
			openIncidents++
		}
	}

	// Monthly ingest counters (0 when the gateway has no DB — cosmetic).
	var errEvents, rumViews int64
	if g.db != nil {
		tenantID := saas.TenantFrom(ctx)
		errEvents = g.monthlyUsage(ctx, tenantID, saas.MetricErrorEvents)
		rumViews = g.monthlyUsage(ctx, tenantID, saas.MetricRUMPageviews)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"overview": map[string]any{
			"monitors":           mon,
			"incidents":          map[string]any{"open": openIncidents},
			"uptime_percent_24h": uptimePct,
			"errors":             map[string]any{"events_this_month": errEvents},
			"rum":                map[string]any{"pageviews_this_month": rumViews},
		},
	})
}
