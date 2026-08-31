package main

// handlers_ci_test.go — /v1/ci/events (sqlmock): ingest + list happy paths,
// quota throttling, and the P0 tenant-isolation invariants.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const otherTenantCIEventID = "cccccccc-dddd-4eee-8fff-aaaaaaaaaaaa"

// expectFreeTierLookup queues the saas.EffectiveLimits query (tier lookup)
// returning the free tier — the standard pre-write quota check.
func expectFreeTierLookup(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now()))
}

func TestIngestCIEventSuccess(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	expectFreeTierLookup(mock)
	mock.ExpectQuery(`INSERT INTO np_saas_usage`).
		WithArgs(testTenant, "ci_events", sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(1)))
	mock.ExpectQuery(`INSERT INTO np_saas_ci_events`).
		WithArgs(testTenant, "nself-org/plugins-pro", "ci.yml", "success", "", "", "").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"}).
			AddRow(otherTenantCIEventID, "nself-org/plugins-pro", "ci.yml", "success", "", "", "", time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/ci/events",
		`{"repo":"nself-org/plugins-pro","workflow":"ci.yml","status":"success"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("ingest = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		CIEvent ciEventDTO `json:"ci_event"`
	}
	decodeBody(t, rec, &env)
	if env.CIEvent.Repo != "nself-org/plugins-pro" || env.CIEvent.Status != "success" {
		t.Errorf("ci_event = %+v", env.CIEvent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestIngestCIEventFailureDispatchesAlert — status=failure must fire an
// alert through the same router path monitor-down uses.
func TestIngestCIEventFailureDispatchesAlert(t *testing.T) {
	db, mock := newSQLMock(t)
	var gotBody []byte
	alertSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer alertSrv.Close()
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", alertSrv.URL)

	expectFreeTierLookup(mock)
	mock.ExpectQuery(`INSERT INTO np_saas_usage`).
		WithArgs(testTenant, "ci_events", sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(1)))
	mock.ExpectQuery(`INSERT INTO np_saas_ci_events`).
		WithArgs(testTenant, "nself-org/plugins-pro", "ci.yml", "failure", "https://ci.example/run/1", "abc123", "build broke").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"}).
			AddRow(otherTenantCIEventID, "nself-org/plugins-pro", "ci.yml", "failure",
				"https://ci.example/run/1", "abc123", "build broke", time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/ci/events",
		`{"repo":"nself-org/plugins-pro","workflow":"ci.yml","status":"failure","run_url":"https://ci.example/run/1","sha":"abc123","title":"build broke"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("ingest failure = %d: %s", rec.Code, rec.Body.String())
	}
	if len(gotBody) == 0 {
		t.Fatal("expected alert-router to receive a dispatched event")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIngestCIEventMissingRepo422(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodPost, "/v1/ci/events", `{"status":"success"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing repo = %d, want 422", rec.Code)
	}
}

func TestIngestCIEventBadStatus422(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodPost, "/v1/ci/events",
		`{"repo":"x/y","status":"maybe"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status = %d, want 422", rec.Code)
	}
}

// TestIngestCIEventQuotaThrottled429 — free-tier cap exhausted -> 429, and
// crucially NO INSERT into np_saas_ci_events (reject before the write).
func TestIngestCIEventQuotaThrottled429(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	expectFreeTierLookup(mock)
	// Free tier cap is 500/month; return a post-increment count already over.
	mock.ExpectQuery(`INSERT INTO np_saas_usage`).
		WithArgs(testTenant, "ci_events", sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(501)))
	// NOTE: no INSERT INTO np_saas_ci_events expectation — a write here fails.

	rec := doReq(t, g, http.MethodPost, "/v1/ci/events",
		`{"repo":"nself-org/plugins-pro","status":"success"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-quota ingest = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestListCIEventsTenantScoped — ISOLATION: the list query is parameterized
// by the VERIFIED tenant only.
func TestListCIEventsTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_saas_ci_events\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"}).
			AddRow(otherTenantCIEventID, "nself-org/plugins-pro", "ci.yml", "failure",
				"https://ci.example/run/1", "abc123", "build broke", time.Now()))

	rec := doReq(t, g, http.MethodGet, "/v1/ci/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		CIEvents []ciEventDTO `json:"ci_events"`
	}
	decodeBody(t, rec, &env)
	if len(env.CIEvents) != 1 || env.CIEvents[0].Repo != "nself-org/plugins-pro" {
		t.Errorf("ci_events = %+v", env.CIEvents)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestListCIEventsEmpty — a tenant with zero events (or another tenant's
// events existing under a different tenant_id) gets an empty list, never
// another tenant's rows — enforced structurally since the query is always
// WHERE tenant_id = $1 with the verified tenant.
func TestListCIEventsEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_saas_ci_events\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"}))

	rec := doReq(t, g, http.MethodGet, "/v1/ci/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		CIEvents []ciEventDTO `json:"ci_events"`
	}
	decodeBody(t, rec, &env)
	if len(env.CIEvents) != 0 {
		t.Errorf("expected empty list, got %+v", env.CIEvents)
	}
}

func TestCIEventsUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/ci/events"},
		{http.MethodPost, "/v1/ci/events"},
	} {
		req, rec := plainRequest(tc.method, tc.path)
		g.router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s unauthenticated = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
