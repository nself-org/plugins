package main

// detector_test.go — down-detector transitions: incident auto-create +
// alert dispatch on DOWN, resolve + monitor.up on recovery, idempotency,
// and tenant isolation (each event carries ONLY its own row's tenant).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const secondTenant = "99999999-8888-4777-8666-555555555555"

// fakePlugins captures incident + alert calls the detector makes.
type fakePlugins struct {
	mu        sync.Mutex
	incidents []capturedCall // POST /incidents/ and /incidents/{id}/resolve
	alerts    []capturedAlert
}

type capturedCall struct {
	Path   string
	Tenant string
	Body   map[string]any
}

type capturedAlert struct {
	Tenant string
	Event  alertEvent
}

func newFakePlugins(t *testing.T) (*fakePlugins, *httptest.Server, *httptest.Server) {
	t.Helper()
	f := &fakePlugins{}
	incident := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.incidents = append(f.incidents, capturedCall{
			Path: r.URL.Path, Tenant: r.Header.Get("X-Hasura-Tenant-Id"), Body: body,
		})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"inc-42","title":"x","state":"open","created_at":"2026-07-03T00:00:00Z"}`))
	}))
	t.Cleanup(incident.Close)
	alert := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev alertEvent
		_ = json.NewDecoder(r.Body).Decode(&ev)
		f.mu.Lock()
		f.alerts = append(f.alerts, capturedAlert{Tenant: r.Header.Get("X-Hasura-Tenant-Id"), Event: ev})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dispatched":1,"routes":[],"delivery":{"sent":1}}`))
	}))
	t.Cleanup(alert.Close)
	return f, incident, alert
}

// sweepCols are the columns sweepQuery returns.
var sweepCols = []string{"id", "tenant_id", "name", "url", "statuses", "status", "incident_id"}

func TestDetectorOpensIncidentAndFiresAlert(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2
	g.cfg.DashboardBaseURL = "https://sentry.nself.org"

	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{down,down,up}", "", ""))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-1", testTenant, 2, "inc-42").
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	if len(f.incidents) != 1 || f.incidents[0].Path != "/incidents/" {
		t.Fatalf("incidents calls = %+v, want one create", f.incidents)
	}
	if f.incidents[0].Tenant != testTenant {
		t.Errorf("incident tenant = %q, want %q", f.incidents[0].Tenant, testTenant)
	}
	if title := f.incidents[0].Body["title"]; title != "Monitor down: API" {
		t.Errorf("incident title = %v", title)
	}
	if len(f.alerts) != 1 {
		t.Fatalf("alerts = %+v, want one monitor.down", f.alerts)
	}
	ev := f.alerts[0]
	if ev.Tenant != testTenant || ev.Event.Kind != "monitor.down" ||
		ev.Event.State != "firing" || ev.Event.DedupID != "mon-1" {
		t.Errorf("alert event wrong: %+v", ev)
	}
	if ev.Event.URL != "https://sentry.nself.org/monitors/mon-1" {
		t.Errorf("alert deep link = %q", ev.Event.URL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDetectorIdempotentWhileDown(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2

	// Already recorded down → no incident, no alert, no state write.
	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{down,down,down}", "down", "inc-42"))

	g.sweepMonitors(context.Background())

	if len(f.incidents) != 0 || len(f.alerts) != 0 {
		t.Fatalf("repeat sweep acted again: incidents=%d alerts=%d", len(f.incidents), len(f.alerts))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDetectorBelowThresholdNoAction(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2

	// One failed check < threshold 2 → nothing happens.
	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{down,up,up}", "", ""))

	g.sweepMonitors(context.Background())

	if len(f.incidents) != 0 || len(f.alerts) != 0 {
		t.Fatalf("acted below threshold: incidents=%d alerts=%d", len(f.incidents), len(f.alerts))
	}
}

func TestDetectorRecoveryResolvesIncident(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2

	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{up,down,down}", "down", "inc-42"))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-1", testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	// The incident state machine forbids open→resolved, so recovery must
	// acknowledge first, then resolve (in that order).
	if len(f.incidents) != 2 ||
		f.incidents[0].Path != "/incidents/inc-42/acknowledge" ||
		f.incidents[1].Path != "/incidents/inc-42/resolve" {
		t.Fatalf("incident calls = %+v, want acknowledge then resolve", f.incidents)
	}
	if f.incidents[0].Tenant != testTenant || f.incidents[1].Tenant != testTenant {
		t.Errorf("recovery incident calls carry wrong tenant: %+v", f.incidents)
	}
	if len(f.alerts) != 1 || f.alerts[0].Event.Kind != "monitor.up" || f.alerts[0].Event.State != "resolved" {
		t.Fatalf("recovery alert wrong: %+v", f.alerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDetectorRecoveryHonorsStateMachine runs the recovery path against a
// stateful fake incident-mgmt that enforces the REAL state machine
// (open→acknowledged→mitigating→resolved; open→resolved is 409). It locks the
// live bug from 2026-07-03 (incident 7da3f891 stuck open): after recovery the
// incident must reach "resolved", not stay open behind a 409.
func TestDetectorRecoveryHonorsStateMachine(t *testing.T) {
	db, mock := newSQLMock(t)

	incState := struct {
		mu    sync.Mutex
		state string
	}{state: "open"}
	incident := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		incState.mu.Lock()
		defer incState.mu.Unlock()
		var to string
		switch r.URL.Path {
		case "/incidents/inc-42/acknowledge":
			to = "acknowledged"
		case "/incidents/inc-42/resolve":
			to = "resolved"
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Mirror internal/state ValidTransitions: open→resolved is forbidden.
		valid := map[string][]string{
			"open":         {"acknowledged"},
			"acknowledged": {"mitigating", "resolved"},
			"mitigating":   {"resolved"},
			"resolved":     {},
		}
		for _, next := range valid[incState.state] {
			if next == to {
				incState.state = to
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"inc-42","state":"` + to + `"}`))
				return
			}
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"invalid state transition"}`))
	}))
	t.Cleanup(incident.Close)

	alert := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"dispatched":1}`))
	}))
	t.Cleanup(alert.Close)

	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2

	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{up,down,down}", "down", "inc-42"))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-1", testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	incState.mu.Lock()
	final := incState.state
	incState.mu.Unlock()
	if final != "resolved" {
		t.Fatalf("incident state after recovery = %q, want resolved (stuck-open bug)", final)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDetectorTenantIsolation locks the P0 rule for detector-originated
// traffic: each monitor's incident + alert carries ONLY the tenant recorded
// on ITS np_uptime_targets row — never another tenant's id.
func TestDetectorTenantIsolation(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2

	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-a", testTenant, "A", "https://a.example.com", "{down,down}", "", "").
			AddRow("mon-b", secondTenant, "B", "https://b.example.com", "{down,down}", "", ""))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-a", testTenant, 2, "inc-42").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-b", secondTenant, 2, "inc-42").WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	if len(f.alerts) != 2 {
		t.Fatalf("alerts = %d, want 2", len(f.alerts))
	}
	byDedup := map[string]string{}
	for _, a := range f.alerts {
		byDedup[a.Event.DedupID] = a.Tenant
	}
	if byDedup["mon-a"] != testTenant || byDedup["mon-b"] != secondTenant {
		t.Errorf("tenant leak across monitors: %+v", byDedup)
	}
	for _, c := range f.incidents {
		if c.Tenant != testTenant && c.Tenant != secondTenant {
			t.Errorf("incident with foreign tenant %q", c.Tenant)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestSSLExpirySweepAlertsOncePerCert verifies the ssl.expiring dispatch and
// the (monitor, not_after) ledger write that suppresses repeats.
func TestSSLExpirySweepAlertsOncePerCert(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.SSLExpiryDays = 14

	notAfter := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`FROM np_uptime_tls tls`).WithArgs(14).
		WillReturnRows(sqlmock.NewRows(
			[]string{"target_id", "tenant_id", "name", "url", "not_after", "days_remaining"}).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", notAfter, 7))
	mock.ExpectExec(`INSERT INTO np_saas_ssl_alerts`).
		WithArgs("mon-1", testTenant, notAfter).
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepSSLExpiry(context.Background())

	if len(f.alerts) != 1 {
		t.Fatalf("ssl alerts = %d, want 1", len(f.alerts))
	}
	ev := f.alerts[0]
	if ev.Tenant != testTenant || ev.Event.Kind != "ssl.expiring" ||
		ev.Event.State != "firing" || ev.Event.DedupID != "mon-1|ssl" {
		t.Errorf("ssl alert wrong: %+v", ev)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestConsecutiveDown(t *testing.T) {
	cases := []struct {
		in   []string
		want int
	}{
		{[]string{"down", "down", "up"}, 2},
		{[]string{"up", "down", "down"}, 0},
		{[]string{"down"}, 1},
		{nil, 0},
	}
	for _, c := range cases {
		if got := consecutiveDown(c.in); got != c.want {
			t.Errorf("consecutiveDown(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
