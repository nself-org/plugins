// Purpose: Route-wiring tests for the idme plugin's cmd/main.
// Inputs: httptest requests against the router built by newRouter, with a mocked pgx pool.
// Outputs: asserts route dispatch (200/404) and the health JSON body.
// Constraints: No real Postgres, no real port binding; exercises newRouter only, never main().
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"github.com/nself-org/nself-idme/internal"
)

func newTestDB(t *testing.T) (*internal.DB, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &internal.DB{Pool: mock}, mock
}

func TestNewRouter_Health(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"service":"idme-plugin"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewRouter_VerificationsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "idme_uuid", "email", "verified", "verification_level",
			"first_name", "last_name", "birth_date", "zip", "phone", "access_token", "refresh_token",
			"token_expires_at", "verified_at", "last_synced_at", "created_at", "updated_at",
		}))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_GroupsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, group_type").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "verification_id", "user_id", "group_type", "group_name",
			"verified", "verified_at", "expires_at", "affiliation", "rank", "status", "created_at", "updated_at",
		}))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_BadgesRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "verification_id", "user_id", "badge_type", "badge_name",
			"badge_icon", "badge_color", "verified_at", "expires_at", "active", "display_order",
			"created_at", "updated_at",
		}))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/badges", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_UserVerificationsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "idme_uuid", "email", "verified", "verification_level",
			"first_name", "last_name", "birth_date", "zip", "phone", "access_token", "refresh_token",
			"token_expires_at", "verified_at", "last_synced_at", "created_at", "updated_at",
		}))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/u1/verifications", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_UserBadgesRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "verification_id", "user_id", "badge_type", "badge_name",
			"badge_icon", "badge_color", "verified_at", "expires_at", "active", "display_order",
			"created_at", "updated_at",
		}))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/u1/badges", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_StatsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	for i := 0; i < 5; i++ {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_").
			WithArgs(pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	}

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_VerificationsCallback_MissingCode(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback", strings.NewReader(`{}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_CreateGroup_MissingFields(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(`{}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_DeleteGroup_RouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectExec("DELETE FROM idme_groups").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/g1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_CreateBadge_MissingFields(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badges", strings.NewReader(`{}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_GetVerification_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	r := newRouter(db, internal.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications/v1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunUntilSignal_GracefulShutdown(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db, internal.Config{})
	srv := newServer(r, internal.Config{Port: "0"})
	srv.Addr = "127.0.0.1:0"

	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runUntilSignal(srv, quit)
	}()

	// Give the listener goroutine a moment to start, then request shutdown.
	time.Sleep(50 * time.Millisecond)
	quit <- syscall.SIGTERM

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runUntilSignal returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runUntilSignal did not return after shutdown signal")
	}
}

func TestNewServer_Fields(t *testing.T) {
	cfg := internal.Config{Port: "4242"}
	handler := http.NewServeMux()
	srv := newServer(handler, cfg)
	if srv.Addr != ":4242" {
		t.Errorf("Addr = %q", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Handler should not be nil")
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
	}
}
