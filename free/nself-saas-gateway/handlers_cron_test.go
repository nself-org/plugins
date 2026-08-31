package main

// handlers_cron_test.go — /v1/cron (sqlmock): list + heartbeat recording and
// the P0 tenant-isolation invariants.

import (
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const cronMonitorID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

func TestListCronTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`to_regclass`).WithArgs("np_cron_monitors").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// ISOLATION: list is parameterized by the VERIFIED tenant.
	mock.ExpectQuery(`FROM np_cron_monitors\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "slug", "schedule", "grace_period_minutes",
			"monitor_status", "last_status", "last_check_in_at", "next_expected_at"}).
			AddRow(cronMonitorID, "nightly backup", "nightly-backup", "0 2 * * *",
				5, "active", "ok", time.Now(), time.Now()))

	rec := doReq(t, g, http.MethodGet, "/v1/cron", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list cron = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Heartbeats []heartbeatDTO `json:"heartbeats"`
	}
	decodeBody(t, rec, &env)
	if len(env.Heartbeats) != 1 || env.Heartbeats[0].Slug != "nightly-backup" {
		t.Errorf("heartbeats = %+v", env.Heartbeats)
	}
	if env.Heartbeats[0].LastStatus != "ok" || env.Heartbeats[0].MonitorStatus != "active" {
		t.Errorf("status fields wrong: %+v", env.Heartbeats[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListCronMissingTableEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`to_regclass`).WithArgs("np_cron_monitors").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	rec := doReq(t, g, http.MethodGet, "/v1/cron", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("missing table = %d, want 200", rec.Code)
	}
}

func TestCronHeartbeatRecorded(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// Ownership check scoped to the verified tenant.
	mock.ExpectQuery(`FROM np_cron_monitors WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(cronMonitorID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(cronMonitorID))
	mock.ExpectExec(`INSERT INTO np_cron_check_ins`).
		WithArgs(cronMonitorID, "ok", nil, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE np_cron_monitors`).
		WithArgs("ok", cronMonitorID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := doReq(t, g, http.MethodPost, "/v1/cron/"+cronMonitorID+"/heartbeat", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("heartbeat = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Heartbeat struct {
			MonitorID string `json:"monitor_id"`
			Status    string `json:"status"`
		} `json:"heartbeat"`
	}
	decodeBody(t, rec, &env)
	if env.Heartbeat.MonitorID != cronMonitorID || env.Heartbeat.Status != "ok" {
		t.Errorf("heartbeat = %+v", env.Heartbeat)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestCronHeartbeatCrossTenant404 — ISOLATION: pinging another tenant's
// monitor must 404 with NO write.
func TestCronHeartbeatCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_cron_monitors WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(cronMonitorID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // not this tenant's
	// NOTE: no INSERT/UPDATE expectations — a write here fails the test.

	rec := doReq(t, g, http.MethodPost, "/v1/cron/"+cronMonitorID+"/heartbeat",
		`{"status":"ok"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant heartbeat = %d, want 404", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCronHeartbeatBadStatus422(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodPost, "/v1/cron/"+cronMonitorID+"/heartbeat",
		`{"status":"maybe"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status = %d, want 422", rec.Code)
	}
}

func TestCronUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req, rec := plainRequest(http.MethodGet, "/v1/cron")
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /v1/cron = %d, want 401", rec.Code)
	}
}
