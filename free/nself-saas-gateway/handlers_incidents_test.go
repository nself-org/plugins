package main

// handlers_incidents_test.go — /v1/incidents mapping against a fake
// nself-incident-mgmt backend.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeIncident(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /incidents/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") == "open" {
			_ = json.NewEncoder(w).Encode(map[string]any{"incidents": []map[string]any{
				{"id": "i-1", "title": "API down", "severity": "critical", "state": "open",
					"created_at": "2026-07-01T10:00:00Z"},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"incidents": []map[string]any{
			{"id": "i-1", "title": "API down", "severity": "critical", "state": "open",
				"created_at": "2026-07-01T10:00:00Z"},
			{"id": "i-2", "title": "Slow queries", "severity": "warning", "state": "mitigating",
				"created_at": "2026-07-01T09:00:00Z", "acknowledged_at": "2026-07-01T09:30:00Z"},
		}})
	})
	mux.HandleFunc("POST /incidents/i-1/acknowledge", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "i-1", "title": "API down", "severity": "critical", "state": "acknowledged",
			"created_at": "2026-07-01T10:00:00Z", "acknowledged_at": "2026-07-01T10:05:00Z"})
	})
	mux.HandleFunc("POST /incidents/i-1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "i-1", "title": "API down", "severity": "critical", "state": "resolved",
			"created_at": "2026-07-01T10:00:00Z", "resolved_at": "2026-07-01T11:00:00Z"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListIncidentsMapsStates(t *testing.T) {
	inc := fakeIncident(t)
	g := newTestGateway(nil, inc.URL, inc.URL, inc.URL)

	rec := doReq(t, g, http.MethodGet, "/v1/incidents/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Incidents []incidentDTO `json:"incidents"`
	}
	decodeBody(t, rec, &env)
	if len(env.Incidents) != 2 {
		t.Fatalf("incidents = %d, want 2", len(env.Incidents))
	}
	if env.Incidents[0].Status != "open" || env.Incidents[0].StartedAt == "" {
		t.Errorf("open incident mapped wrong: %+v", env.Incidents[0])
	}
	// mitigating folds into the contract's "acknowledged".
	if env.Incidents[1].Status != "acknowledged" {
		t.Errorf("mitigating → %q, want acknowledged", env.Incidents[1].Status)
	}
}

func TestListIncidentsStatusFilter(t *testing.T) {
	inc := fakeIncident(t)
	g := newTestGateway(nil, inc.URL, inc.URL, inc.URL)

	rec := doReq(t, g, http.MethodGet, "/v1/incidents/?status=open", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered list = %d", rec.Code)
	}
	var env struct {
		Incidents []incidentDTO `json:"incidents"`
	}
	decodeBody(t, rec, &env)
	if len(env.Incidents) != 1 || env.Incidents[0].Status != "open" {
		t.Errorf("filter passthrough wrong: %+v", env.Incidents)
	}
}

func TestAckAndResolveIncident(t *testing.T) {
	inc := fakeIncident(t)
	g := newTestGateway(nil, inc.URL, inc.URL, inc.URL)

	rec := doReq(t, g, http.MethodPost, "/v1/incidents/i-1/ack", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ack = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Incident incidentDTO `json:"incident"`
	}
	decodeBody(t, rec, &env)
	if env.Incident.Status != "acknowledged" || env.Incident.AcknowledgedAt == "" {
		t.Errorf("ack mapping wrong: %+v", env.Incident)
	}

	rec = doReq(t, g, http.MethodPost, "/v1/incidents/i-1/resolve", "")
	decodeBody(t, rec, &env)
	if env.Incident.Status != "resolved" || env.Incident.ResolvedAt == "" {
		t.Errorf("resolve mapping wrong: %+v", env.Incident)
	}
}
