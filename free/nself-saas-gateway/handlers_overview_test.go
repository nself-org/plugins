package main

// handlers_overview_test.go — /v1/overview aggregation over fake upstreams.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeOverviewUpstreams serves targets/results (uptime) and incidents.
func fakeOverviewUpstreams(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/targets/":
			_, _ = w.Write([]byte(`{"targets":[
				{"id":"t-1","name":"one","url":"https://a","protocol":"https","interval_secs":300,"enabled":true,"created_at":"2026-07-01T00:00:00Z"},
				{"id":"t-2","name":"two","url":"https://b","protocol":"https","interval_secs":300,"enabled":true,"created_at":"2026-07-01T00:00:00Z"},
				{"id":"t-3","name":"three","url":"https://c","protocol":"https","interval_secs":300,"enabled":false,"created_at":"2026-07-01T00:00:00Z"},
				{"id":"t-4","name":"four","url":"https://d","protocol":"https","interval_secs":300,"enabled":true,"created_at":"2026-07-01T00:00:00Z"}]}`))
		case "/api/v1/results":
			_, _ = w.Write([]byte(`{"results":[
				{"target_id":"t-1","status":"up"},
				{"target_id":"t-2","status":"down"},
				{"target_id":"t-1","status":"up"},
				{"target_id":"t-2","status":"up"}]}`))
		case "/incidents/":
			_, _ = w.Write([]byte(`{"incidents":[
				{"id":"i-1","title":"a","severity":"major","state":"open","created_at":"2026-07-01T00:00:00Z"},
				{"id":"i-2","title":"b","severity":"minor","state":"mitigating","created_at":"2026-07-01T00:00:00Z"},
				{"id":"i-3","title":"c","severity":"minor","state":"resolved","created_at":"2026-07-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestOverviewAggregates(t *testing.T) {
	upstream := fakeOverviewUpstreams(t)
	defer upstream.Close()

	g := newTestGateway(nil, upstream.URL, upstream.URL, upstream.URL)
	rec := doReq(t, g, http.MethodGet, "/v1/overview", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("overview = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Overview struct {
			Monitors struct {
				Total, Up, Down, Paused, Pending int
			} `json:"monitors"`
			Incidents struct {
				Open int `json:"open"`
			} `json:"incidents"`
			UptimePct *float64 `json:"uptime_percent_24h"`
			Errors    struct {
				EventsThisMonth int64 `json:"events_this_month"`
			} `json:"errors"`
		} `json:"overview"`
	}
	decodeBody(t, rec, &resp)

	o := resp.Overview
	// t-1 up, t-2 down (newest result first), t-3 paused, t-4 no result → pending.
	if o.Monitors.Total != 4 || o.Monitors.Up != 1 || o.Monitors.Down != 1 || o.Monitors.Paused != 1 || o.Monitors.Pending != 1 {
		t.Errorf("monitors = %+v, want total=4 up=1 down=1 paused=1 pending=1", o.Monitors)
	}
	// open + mitigating count as open; resolved does not.
	if o.Incidents.Open != 2 {
		t.Errorf("incidents.open = %d, want 2", o.Incidents.Open)
	}
	// 3 up / 4 probes = 75%.
	if o.UptimePct == nil || *o.UptimePct != 75.0 {
		t.Errorf("uptime_percent_24h = %v, want 75.0", o.UptimePct)
	}
	// No DB in this harness → counters are zero, not an error.
	if o.Errors.EventsThisMonth != 0 {
		t.Errorf("errors.events_this_month = %d, want 0", o.Errors.EventsThisMonth)
	}
}

func TestOverviewRequiresAuth(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/v1/overview", nil)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated overview = %d, want 401", rec.Code)
	}
}
