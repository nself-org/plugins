// Purpose: Shared test helpers for the compliance internal package's handler tests.
// Inputs: n/a (helper constructors only).
// Outputs: mock *DB backed by pgxmock, chi-route-param request builder.
// Constraints: No real Postgres; pgxmock.PgxPoolIface satisfies internal.PgxIface.
package internal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v3"
)

// errBoom is a stand-in DB error for exercising handler error branches.
var errBoom = errors.New("boom")

// nowTime returns a fixed-ish timestamp for row fixtures.
func nowTime() time.Time { return time.Now().UTC() }

// newMockDB returns a *DB backed by a pgxmock pool, and the mock itself for
// setting expectations.
func newMockDB(t *testing.T) (*DB, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &DB{Pool: mock}, mock
}

// reqWithChiParam builds an httptest.Request carrying a chi URL param (e.g. "id").
func reqWithChiParam(method, url, body, key, val string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, bytes.NewBufferString(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}
