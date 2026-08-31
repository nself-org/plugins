package main

// handlers_incidents_crud_test.go — /v1/incidents create/get/patch against a
// fake nself-incident-mgmt backend, including the P0 tenant seam: the
// upstream hop must carry the VERIFIED tenant (service JWT + injected
// header), never the client's session JWT or a spoofed header.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// fakeIncidentCRUD is a fake incident plugin recording the tenant it saw.
func fakeIncidentCRUD(t *testing.T, sawTenant *string, sawAuth *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	record := func(r *http.Request) {
		*sawTenant = r.Header.Get("X-Hasura-Tenant-Id")
		*sawAuth = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	mux.HandleFunc("POST /incidents/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "i-9", "title": body["title"], "severity": body["severity"],
			"state": "open", "created_at": "2026-07-03T10:00:00Z"})
	})
	mux.HandleFunc("GET /incidents/i-9/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "i-9", "title": "DB down", "description": "primary unreachable",
			"severity": "high", "state": "mitigating",
			"created_at":      "2026-07-03T10:00:00Z",
			"acknowledged_at": "2026-07-03T10:05:00Z",
			"timeline": []map[string]any{
				{"id": "t-1", "kind": "created", "message": "incident opened",
					"actor": "detector", "occurred_at": "2026-07-03T10:00:00Z"},
				{"id": "t-2", "kind": "state_change", "message": "acknowledged",
					"occurred_at": "2026-07-03T10:05:00Z"},
			}})
	})
	mux.HandleFunc("GET /incidents/i-other/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		// Upstream tenant scoping: not this tenant's incident.
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})
	mux.HandleFunc("PATCH /incidents/i-9/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		state, _ := body["state"].(string)
		if state == "" {
			state = "open"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "i-9", "title": "DB down", "severity": "high", "state": state,
			"created_at": "2026-07-03T10:00:00Z"})
	})
	return httptest.NewServer(mux)
}

func TestCreateIncident(t *testing.T) {
	var sawTenant, sawAuth string
	up := fakeIncidentCRUD(t, &sawTenant, &sawAuth)
	defer up.Close()
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodPost, "/v1/incidents",
		`{"title":"DB down","description":"primary unreachable","severity":"high"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create incident = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Incident incidentDTO `json:"incident"`
	}
	decodeBody(t, rec, &env)
	if env.Incident.ID != "i-9" || env.Incident.Status != "open" {
		t.Errorf("incident = %+v", env.Incident)
	}
	// P0 seam: upstream saw the VERIFIED tenant via minted service JWT.
	if sawTenant != testTenant {
		t.Errorf("upstream tenant = %q, want %q", sawTenant, testTenant)
	}
	if tid, err := saas.VerifyJWTTenant(sawAuth, []byte(testUpstreamSecret)); err != nil || tid != testTenant {
		t.Errorf("upstream service JWT tenant = %q err=%v", tid, err)
	}
}

func TestCreateIncidentValidation(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for body, want := range map[string]int{
		`{"title":""}`:                          http.StatusUnprocessableEntity,
		`{"title":"x","severity":"apocalypse"}`: http.StatusUnprocessableEntity,
		`not json`:                              http.StatusBadRequest,
	} {
		rec := doReq(t, g, http.MethodPost, "/v1/incidents", body)
		if rec.Code != want {
			t.Errorf("create %q = %d, want %d", body, rec.Code, want)
		}
	}
}

func TestGetIncidentWithTimeline(t *testing.T) {
	var sawTenant, sawAuth string
	up := fakeIncidentCRUD(t, &sawTenant, &sawAuth)
	defer up.Close()
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodGet, "/v1/incidents/i-9", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get incident = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Incident    incidentDTO        `json:"incident"`
		Description string             `json:"description"`
		Updates     []timelineEntryDTO `json:"updates"`
	}
	decodeBody(t, rec, &env)
	if env.Incident.Status != "acknowledged" { // mitigating folds to acknowledged
		t.Errorf("status = %q, want acknowledged", env.Incident.Status)
	}
	if env.Description != "primary unreachable" || len(env.Updates) != 2 {
		t.Errorf("detail = %q updates=%d", env.Description, len(env.Updates))
	}
	if env.Updates[0].Actor != "detector" || env.Updates[1].Actor != "" {
		t.Errorf("updates = %+v", env.Updates)
	}
	if sawTenant != testTenant {
		t.Errorf("upstream tenant = %q, want %q", sawTenant, testTenant)
	}
}

// TestGetIncidentCrossTenant404 — ISOLATION: the plugin scopes by the
// service-JWT tenant; its 404 must relay as the contract 404 envelope.
func TestGetIncidentCrossTenant404(t *testing.T) {
	var sawTenant, sawAuth string
	up := fakeIncidentCRUD(t, &sawTenant, &sawAuth)
	defer up.Close()
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodGet, "/v1/incidents/i-other", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant incident = %d, want 404", rec.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, rec, &env)
	if env.Error.Code != "not_found" {
		t.Errorf("error code = %q, want not_found", env.Error.Code)
	}
}

func TestPatchIncidentStatus(t *testing.T) {
	var sawTenant, sawAuth string
	up := fakeIncidentCRUD(t, &sawTenant, &sawAuth)
	defer up.Close()
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodPatch, "/v1/incidents/i-9", `{"status":"resolved"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch incident = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Incident incidentDTO `json:"incident"`
	}
	decodeBody(t, rec, &env)
	if env.Incident.Status != "resolved" {
		t.Errorf("status = %q, want resolved", env.Incident.Status)
	}
	if sawTenant != testTenant {
		t.Errorf("upstream tenant = %q, want %q", sawTenant, testTenant)
	}
}

func TestPatchIncidentValidation(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for body, want := range map[string]int{
		`{}`:                        http.StatusUnprocessableEntity, // nothing to update
		`{"status":"mitigating"}`:   http.StatusUnprocessableEntity, // not a contract status
		`{"severity":"apocalypse"}`: http.StatusUnprocessableEntity,
	} {
		rec := doReq(t, g, http.MethodPatch, "/v1/incidents/i-9", body)
		if rec.Code != want {
			t.Errorf("patch %q = %d, want %d", body, rec.Code, want)
		}
	}
}
