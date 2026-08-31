package main

// handlers_statuspublic.go — GET /v1/status/public/{slug}: the ONE
// intentionally-public read on the SaaS surface (rendered by the SPA at
// sentry.nself.org/s/<slug>).
//
// Purpose: multi-tenant public status pages from shared infra. The self-host
//   nself-status-page plugin is one-instance-per-page by design; the SaaS
//   needs N public pages, so the gateway renders them itself: slug →
//   np_saas_status_pages registry row (tenant_id) → that tenant's uptime
//   monitors as components + active/recent incidents, via the same loopback
//   plugin calls the authed dashboard uses (service JWT minted for the
//   RESOLVED tenant — never from anything the caller sent).
// Outputs: {"status_page":{title, slug, overall_status, components:[{id,
//   name, status, uptime_percent}], incidents:[{title,status,started_at,
//   resolved_at?}], generated_at}} — snake_case like the rest of /v1.
// Constraints — FAIL-CLOSED (P0 rules):
//   - unknown slug, non-public page, invalid slug → the SAME generic 404
//     (no tenant enumeration, no existence leak).
//   - the caller's Authorization / any header is NEVER forwarded upstream;
//     tenant identity comes ONLY from the registry row.
//   - private components never appear: monitors probing internal-shaped
//     hosts (loopback/RFC1918/link-local IPs, dotless docker service names
//     like "postgres"/"redis", *.local/*.internal/... suffixes) are private
//     by default — the nself-status-page visibility semantics — and a
//     "[private]" name marker hides any monitor; "[public]" opts one in.
//   - monitor URLs, tenant ids, emails, latency internals are never emitted;
//     a public component is {id, name, status, uptime_percent} only.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// publicComponentDTO is one public status-page component (a public monitor).
// Deliberately minimal: never carries the monitor URL or any internal field.
type publicComponentDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"` // operational | down | unknown
	UptimePercent *float64 `json:"uptime_percent"`
}

// publicIncidentDTO is one public incident entry (title + lifecycle only).
type publicIncidentDTO struct {
	Title      string `json:"title"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// recentIncidentWindow bounds how long resolved incidents stay visible.
const recentIncidentWindow = 7 * 24 * time.Hour

// maxPublicIncidents caps the public incident list.
const maxPublicIncidents = 10

// privateHostSuffixes are hostname suffixes that mark an internal network
// probe (mirrors the nself-status-page "internal service" notion).
var privateHostSuffixes = []string{
	".local", ".localhost", ".internal", ".intranet", ".lan", ".home", ".corp", ".test",
}

// monitorPublic reports whether a monitor may appear on a public status page.
// Fail-closed: unparseable/empty hosts are private. Explicit name markers win:
// "[private]" always hides, "[public]" opts an internal probe in.
func monitorPublic(t upstreamTarget) bool {
	name := strings.ToLower(t.Name)
	if strings.Contains(name, "[private]") {
		return false
	}
	if strings.Contains(name, "[public]") {
		return true
	}
	return !internalHost(hostOf(t.URL))
}

// hostOf extracts the hostname from a monitor URL. Handles scheme-less
// tcp/ping targets ("db.internal:5432", "10.0.0.5").
func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	// Scheme-less: strip any path, then any :port.
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return h
	}
	return raw
}

// internalHost reports whether host is internal-shaped (private by default,
// per the nself-status-page internal-service semantics). Empty → true.
func internalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified()
	}
	if !strings.Contains(host, ".") {
		return true // dotless docker service names: postgres, redis, hasura...
	}
	for _, suffix := range privateHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// publicComponentStatus maps the derived monitor status onto the public enum.
func publicComponentStatus(s string) string {
	switch s {
	case "up":
		return "operational"
	case "down":
		return "down"
	}
	return "unknown" // paused / pending / no probe yet
}

// overallStatus derives the page banner from the component statuses:
// all probed components down → "down"; any down → "degraded"; else
// "operational" (unknowns never degrade the banner).
func overallStatus(components []publicComponentDTO) string {
	var down, probed int
	for _, c := range components {
		switch c.Status {
		case "down":
			down++
			probed++
		case "operational":
			probed++
		}
	}
	switch {
	case down == 0:
		return "operational"
	case down == probed:
		return "down"
	default:
		return "degraded"
	}
}

// publicUpstreamRequest builds the sanitized request used for the
// slug-resolved tenant's loopback plugin calls. SECURITY: headers are
// dropped wholesale so no caller-supplied credential (Authorization,
// X-API-Key, X-Hasura-Tenant-Id, cookies) can ever reach a plugin; the
// tenant in the context is the REGISTRY row's, which makes callUpstream
// mint a service JWT for exactly that tenant.
func publicUpstreamRequest(r *http.Request, tenantID string) *http.Request {
	pr := r.Clone(saas.WithTenant(r.Context(), tenantID))
	pr.Header = http.Header{}
	return pr
}

// writePublicNotFound is the single generic 404 for every miss (unknown
// slug, private page, malformed slug) — indistinguishable on purpose.
func writePublicNotFound(w http.ResponseWriter) {
	writeErr(w, http.StatusNotFound, "not_found", "status page not found")
}

// handlePublicStatus — GET /v1/status/public/{slug} (PUBLIC, no auth).
func (g *gateway) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "slug")))
	if !slugPattern.MatchString(slug) {
		writePublicNotFound(w) // malformed = same generic 404
		return
	}

	// Resolve slug → owning tenant. Only PUBLIC pages resolve; a page the
	// tenant unpublished 404s identically to one that never existed.
	var pageID, tenantID, title string
	err := g.db.QueryRowContext(r.Context(), `
		SELECT id::text, tenant_id::text, name
		FROM np_saas_status_pages
		WHERE slug = $1 AND public = true`, slug,
	).Scan(&pageID, &tenantID, &title)
	if errors.Is(err, sql.ErrNoRows) {
		writePublicNotFound(w)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "status page lookup failed")
		return
	}

	pr := publicUpstreamRequest(r, tenantID)

	// Monitors → components (public ones only).
	status, body, err := g.callUpstream(pr.Context(), pr, http.MethodGet, g.cfg.UptimeURL, "/api/v1/targets/", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "status data unavailable")
		return
	}
	if !upstreamOK(status) {
		writeErr(w, http.StatusBadGateway, "upstream_error", "status data unavailable")
		return
	}
	var targetsEnv struct {
		Targets []upstreamTarget `json:"targets"`
	}
	if err := json.Unmarshal(body, &targetsEnv); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "status data unavailable")
		return
	}

	// Explicit curation (np_saas_status_page_components) wins over the
	// host-shape heuristic: when the page has component rows, show EXACTLY
	// the public=true ones ("[private]" name markers still hide — belt and
	// braces). No rows → legacy heuristic (monitorPublic).
	curated, curatedMode := g.pageComponentCuration(r, pageID, tenantID)

	lastByTarget, uptimeByTarget := g.publicProbeStats(pr)
	components := make([]publicComponentDTO, 0, len(targetsEnv.Targets))
	for _, t := range targetsEnv.Targets {
		name := t.Name
		if curatedMode {
			cur, listed := curated[t.ID]
			if !listed || !cur.public || strings.Contains(strings.ToLower(t.Name), "[private]") {
				continue
			}
			if cur.name != "" {
				name = cur.name
			}
		} else if !monitorPublic(t) {
			continue
		}
		components = append(components, publicComponentDTO{
			ID:            t.ID,
			Name:          strings.TrimSpace(strings.ReplaceAll(name, "[public]", "")),
			Status:        publicComponentStatus(monitorStatus(t, lastByTarget[t.ID])),
			UptimePercent: uptimeByTarget[t.ID],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status_page": map[string]any{
			"title":          title,
			"slug":           slug,
			"overall_status": overallStatus(components),
			"components":     components,
			"incidents":      g.publicIncidents(pr),
			"generated_at":   time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// curatedComponent is one np_saas_status_page_components row (render fields).
type curatedComponent struct {
	name   string
	public bool
}

// pageComponentCuration loads the page's explicit component rows. Returns
// (map monitor_id → row, true) when the page is curated; (nil, false) when
// it has no rows (heuristic mode). FAIL-CLOSED: a query error is treated as
// curated-with-nothing-public rather than falling back to the heuristic.
func (g *gateway) pageComponentCuration(r *http.Request, pageID, tenantID string) (map[string]curatedComponent, bool) {
	rows, err := g.db.QueryContext(r.Context(), `
		SELECT monitor_id, name, public
		FROM np_saas_status_page_components
		WHERE status_page_id = $1 AND tenant_id = $2`, pageID, tenantID)
	if err != nil {
		return map[string]curatedComponent{}, true // fail-closed
	}
	defer rows.Close() //nolint:errcheck

	curated := map[string]curatedComponent{}
	for rows.Next() {
		var monitorID string
		var c curatedComponent
		if err := rows.Scan(&monitorID, &c.name, &c.public); err != nil {
			return map[string]curatedComponent{}, true // fail-closed
		}
		curated[monitorID] = c
	}
	if rows.Err() != nil {
		return map[string]curatedComponent{}, true // fail-closed
	}
	if len(curated) == 0 {
		return nil, false // uncurated page → heuristic
	}
	return curated, true
}

// publicProbeStats fetches the recent probe-result window once and reduces it
// to (newest status, uptime %) per target. Best-effort: on any failure both
// maps are empty and components render "unknown" with null uptime.
func (g *gateway) publicProbeStats(pr *http.Request) (map[string]string, map[string]*float64) {
	last := map[string]string{}
	uptime := map[string]*float64{}
	status, body, err := g.callUpstream(pr.Context(), pr, http.MethodGet, g.cfg.UptimeURL, "/api/v1/results?limit=500", nil)
	if err != nil || !upstreamOK(status) {
		return last, uptime
	}
	var env struct {
		Results []upstreamResult `json:"results"`
	}
	if json.Unmarshal(body, &env) != nil {
		return last, uptime
	}
	type counts struct{ up, total int }
	byTarget := map[string]*counts{}
	for _, res := range env.Results {
		if _, seen := last[res.TargetID]; !seen {
			last[res.TargetID] = res.Status // results arrive newest-first
		}
		c := byTarget[res.TargetID]
		if c == nil {
			c = &counts{}
			byTarget[res.TargetID] = c
		}
		switch res.Status {
		case "up":
			c.up++
			c.total++
		case "down":
			c.total++
		}
	}
	for id, c := range byTarget {
		if c.total == 0 {
			continue
		}
		pct := math.Round(float64(c.up)/float64(c.total)*10000) / 100
		uptime[id] = &pct
	}
	return last, uptime
}

// publicIncidents returns the tenant's active + recently-resolved incidents
// in public shape (title/status/timestamps only — no ids, no internals).
// Best-effort: an incident-plugin failure yields an empty list rather than
// taking the whole public page down.
func (g *gateway) publicIncidents(pr *http.Request) []publicIncidentDTO {
	out := make([]publicIncidentDTO, 0, maxPublicIncidents)
	status, body, err := g.callUpstream(pr.Context(), pr, http.MethodGet, g.cfg.IncidentURL, "/incidents/", nil)
	if err != nil || !upstreamOK(status) {
		return out
	}
	var env struct {
		Incidents []upstreamIncident `json:"incidents"`
	}
	if json.Unmarshal(body, &env) != nil {
		return out
	}
	cutoff := time.Now().UTC().Add(-recentIncidentWindow)
	for _, in := range env.Incidents {
		if in.State == "resolved" && (in.ResolvedAt == nil || in.ResolvedAt.Before(cutoff)) {
			continue
		}
		dto := publicIncidentDTO{
			Title:     in.Title,
			Status:    contractIncidentStatus(in.State),
			StartedAt: in.CreatedAt.UTC().Format(time.RFC3339),
		}
		if in.ResolvedAt != nil {
			dto.ResolvedAt = in.ResolvedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, dto)
		if len(out) == maxPublicIncidents {
			break
		}
	}
	return out
}
