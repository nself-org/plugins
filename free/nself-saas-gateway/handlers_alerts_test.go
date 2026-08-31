package main

// handlers_alerts_test.go — /v1/alerts/channels CRUD over np_alert_channels
// (sqlmock) + the ingest-backed test endpoint (fake alert-router), including
// tier gating and the P0 tenant-isolation invariants.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const channelID = "12121212-3434-4565-8787-909090909090"

func TestListAlertChannelsTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`to_regclass`).WithArgs("np_alert_channels").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`FROM np_alert_channels\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "target", "enabled", "created_at"}).
			AddRow(channelID, "email", "ops@example.com", true, time.Now()))

	rec := doReq(t, g, http.MethodGet, "/v1/alerts/channels", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list channels = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Channels []channelDTO `json:"channels"`
	}
	decodeBody(t, rec, &env)
	if len(env.Channels) != 1 || env.Channels[0].Kind != "email" {
		t.Errorf("channels = %+v", env.Channels)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCreateEmailChannelAllTiers(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// Email needs NO tier lookup — insert directly, scoped to the tenant.
	mock.ExpectQuery(`INSERT INTO np_alert_channels`).
		WithArgs(testTenant, "email", "ops@example.com", "", `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "target", "enabled", "created_at"}).
			AddRow(channelID, "email", "ops@example.com", true, time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels",
		`{"kind":"email","target":"ops@example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create email channel = %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestCreateWebhookChannelFreeTier402 — TIER GATE: free tier has
// WebhookChannels=false → 402 with the quota_exceeded envelope, no insert.
func TestCreateWebhookChannelFreeTier402(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now()))
	// NOTE: no INSERT expectation — a write here fails the test.

	rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels",
		`{"kind":"webhook","target":"https://hooks.example.com/x"}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("free-tier webhook = %d, want 402: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, rec, &env)
	if env.Error.Code != "quota_exceeded" {
		t.Errorf("error code = %q, want quota_exceeded", env.Error.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCreateWebhookChannelBundleTier(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now()))
	mock.ExpectQuery(`INSERT INTO np_alert_channels`).
		WithArgs(testTenant, "webhook", "https://hooks.example.com/x", "s3cret", `{}`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "target", "enabled", "created_at"}).
			AddRow(channelID, "webhook", "https://hooks.example.com/x", true, time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels",
		`{"kind":"webhook","target":"https://hooks.example.com/x","secret":"s3cret"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bundle webhook = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateChannelValidation(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for body, want := range map[string]int{
		`{"kind":"pager","target":"x"}`:                 http.StatusUnprocessableEntity,
		`{"kind":"email","target":"not-an-email"}`:      http.StatusUnprocessableEntity,
		`{"kind":"slack","target":"http://insecure.x"}`: http.StatusUnprocessableEntity,
		`nope`: http.StatusBadRequest,
	} {
		rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels", body)
		if rec.Code != want {
			t.Errorf("create %q = %d, want %d", body, rec.Code, want)
		}
	}
}

// TestDeleteChannelCrossTenant404 — ISOLATION: deleting another tenant's
// channel affects zero rows → 404.
func TestDeleteChannelCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectExec(`DELETE FROM np_alert_channels WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(channelID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rec := doReq(t, g, http.MethodDelete, "/v1/alerts/channels/"+channelID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant delete = %d, want 404", rec.Code)
	}
}

func TestDeleteChannelOwn(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectExec(`DELETE FROM np_alert_channels`).
		WithArgs(channelID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))
	rec := doReq(t, g, http.MethodDelete, "/v1/alerts/channels/"+channelID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
}

// TestTestChannelDispatches — the test endpoint pushes a real event through
// the alert-router ingest with the VERIFIED tenant on the internal hop.
func TestTestChannelDispatches(t *testing.T) {
	var sawTenant string
	var sawEvent map[string]any
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/alerts" {
			http.NotFound(w, r)
			return
		}
		sawTenant = r.Header.Get("X-Hasura-Tenant-Id")
		_ = json.NewDecoder(r.Body).Decode(&sawEvent)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dispatched": 0,
			"routes":     []any{},
			"delivery":   map[string]int{"sent": 1, "suppressed": 0, "failed": 0, "gated": 0},
		})
	}))
	defer router.Close()

	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", router.URL)

	mock.ExpectQuery(`SELECT kind, enabled FROM np_alert_channels WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(channelID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "enabled"}).AddRow("email", true))

	rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels/"+channelID+"/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test channel = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Delivered bool   `json:"delivered"`
		Detail    string `json:"detail"`
	}
	decodeBody(t, rec, &env)
	if !env.Delivered {
		t.Errorf("delivered = false: %s", env.Detail)
	}
	if sawTenant != testTenant {
		t.Errorf("ingest tenant = %q, want %q (internal hop only)", sawTenant, testTenant)
	}
	if sawEvent["kind"] != "channel.test" || sawEvent["dedup_id"] == "" {
		t.Errorf("ingest event = %+v", sawEvent)
	}
}

// TestTestChannelCrossTenant404 — ISOLATION: testing another tenant's
// channel 404s with no dispatch.
func TestTestChannelCrossTenant404(t *testing.T) {
	dispatched := false
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		dispatched = true
		w.WriteHeader(http.StatusOK)
	}))
	defer router.Close()

	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", router.URL)
	mock.ExpectQuery(`SELECT kind, enabled FROM np_alert_channels`).
		WithArgs(channelID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "enabled"})) // not this tenant's

	rec := doReq(t, g, http.MethodPost, "/v1/alerts/channels/"+channelID+"/test", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant test = %d, want 404", rec.Code)
	}
	if dispatched {
		t.Error("test alert was dispatched for a cross-tenant channel")
	}
}

func TestAlertChannelsUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req, rec := plainRequest(http.MethodGet, "/v1/alerts/channels")
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated channels = %d, want 401", rec.Code)
	}
}
