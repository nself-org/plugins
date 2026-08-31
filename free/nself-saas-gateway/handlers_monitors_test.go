package main

// handlers_monitors_test.go — /v1/monitors mapping against a fake
// nself-uptime-monitor backend.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeUptime is a minimal stand-in for nself-uptime-monitor's API.
func fakeUptime(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/targets/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hasura-Tenant-Id") != testTenant {
			http.Error(w, `{"error":"tenant header missing"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"targets": []map[string]any{
			{"id": "t-1", "name": "api", "url": "https://api.example.com", "protocol": "http",
				"interval_secs": 60, "enabled": true, "created_at": "2026-07-01T00:00:00Z"},
			{"id": "t-2", "name": "db", "url": "tcp://db:5432", "protocol": "tcp",
				"interval_secs": 300, "enabled": false, "created_at": "2026-07-01T00:00:00Z"},
		}})
	})
	mux.HandleFunc("GET /api/v1/targets/t-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "t-1", "name": "api", "url": "https://api.example.com", "protocol": "http",
			"interval_secs": 60, "enabled": true, "created_at": "2026-07-01T00:00:00Z"})
	})
	mux.HandleFunc("GET /api/v1/results", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"target_id": "t-1", "status": "up"},   // newest first
			{"target_id": "t-1", "status": "down"}, // older — must be ignored
		}})
	})
	mux.HandleFunc("POST /api/v1/targets/", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["protocol"] != "http" || req["interval_secs"] != float64(60) {
			http.Error(w, `{"error":"bad mapping"}`, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "name": req["name"]})
	})
	mux.HandleFunc("POST /api/v1/targets/t-1/pause", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "enabled": false})
	})
	mux.HandleFunc("POST /api/v1/targets/t-1/resume", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "enabled": true})
	})
	mux.HandleFunc("PUT /api/v1/targets/t-1", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		// Only the edited fields should be forwarded (partial update).
		if _, hasURL := req["url"]; hasURL {
			http.Error(w, `{"error":"url should not be forwarded for a name-only edit"}`, http.StatusBadRequest)
			return
		}
		if req["name"] != "api-renamed" {
			http.Error(w, `{"error":"name not forwarded"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "name": "api-renamed"})
	})
	mux.HandleFunc("DELETE /api/v1/targets/t-9", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("DELETE /api/v1/targets/t-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListMonitorsMapping(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodGet, "/v1/monitors/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Monitors []monitorDTO `json:"monitors"`
	}
	decodeBody(t, rec, &env)
	if len(env.Monitors) != 2 {
		t.Fatalf("monitors = %d, want 2", len(env.Monitors))
	}
	m := env.Monitors[0]
	if m.ID != "t-1" || m.Kind != "http" || m.IntervalSeconds != 60 {
		t.Errorf("field mapping wrong: %+v", m)
	}
	if m.Status != "up" || m.Paused {
		t.Errorf("t-1 status = %q paused=%v, want up/false (newest result wins)", m.Status, m.Paused)
	}
	if env.Monitors[1].Status != "paused" || !env.Monitors[1].Paused {
		t.Errorf("disabled target must be paused: %+v", env.Monitors[1])
	}
}

func TestCreateMonitorMapsAndReturnsFullRecord(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodPost, "/v1/monitors/",
		`{"name":"api","url":"https://api.example.com","kind":"http","interval_seconds":60}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Monitor monitorDTO `json:"monitor"`
	}
	decodeBody(t, rec, &env)
	if env.Monitor.ID != "t-1" || env.Monitor.URL != "https://api.example.com" {
		t.Errorf("create envelope wrong: %+v", env.Monitor)
	}
}

func TestPauseMonitor(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodPost, "/v1/monitors/t-1/pause", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pause = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Monitor monitorDTO `json:"monitor"`
	}
	decodeBody(t, rec, &env)
	if env.Monitor.ID != "t-1" {
		t.Errorf("pause envelope wrong: %+v", env.Monitor)
	}
}

func TestUpdateMonitorPartialEdit(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	// Name-only PATCH must forward ONLY name (not url) to the plugin PUT.
	rec := doReq(t, g, http.MethodPatch, "/v1/monitors/t-1", `{"name":"api-renamed"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Monitor monitorDTO `json:"monitor"`
	}
	decodeBody(t, rec, &env)
	if env.Monitor.ID != "t-1" {
		t.Errorf("patch envelope wrong: %+v", env.Monitor)
	}
}

func TestUpdateMonitorPausedTransition(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	// paused=false → resume endpoint; paused=true → pause endpoint.
	if rec := doReq(t, g, http.MethodPatch, "/v1/monitors/t-1", `{"paused":false}`); rec.Code != http.StatusOK {
		t.Fatalf("resume patch = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doReq(t, g, http.MethodPatch, "/v1/monitors/t-1", `{"paused":true}`); rec.Code != http.StatusOK {
		t.Fatalf("pause patch = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteMonitor(t *testing.T) {
	up := fakeUptime(t)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	if rec := doReq(t, g, http.MethodDelete, "/v1/monitors/t-1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}
	// Upstream 404 must relay as the contract envelope.
	rec := doReq(t, g, http.MethodDelete, "/v1/monitors/t-9", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing = %d, want 404", rec.Code)
	}
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	decodeBody(t, rec, &env)
	if env.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", env.Error.Code)
	}
}

func TestQuota402PassesThrough(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/targets/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"quota_exceeded","message":"Your free tier allows 10 monitors. Upgrade."}`))
	})
	up := httptest.NewServer(mux)
	t.Cleanup(up.Close)
	g := newTestGateway(nil, up.URL, up.URL, up.URL)

	rec := doReq(t, g, http.MethodPost, "/v1/monitors/",
		`{"name":"x","url":"https://x","kind":"http","interval_seconds":60}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("quota relay = %d, want 402", rec.Code)
	}
	var env struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	decodeBody(t, rec, &env)
	if env.Error.Code != "quota_exceeded" || env.Error.Message == "" {
		t.Errorf("quota envelope wrong: %s", rec.Body.String())
	}
}
