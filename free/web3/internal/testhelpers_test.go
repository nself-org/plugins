// Purpose: Shared test helpers for the web3 internal package's handler tests.
// Inputs: n/a (helper constructors only).
// Outputs: mock Handlers + pgxmock pool, chi-route-param request builder.
// Constraints: No real Postgres; pgxmock.PgxPoolIface satisfies internal.PgxIface.
package internal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v3"
)

// errBoom is a stand-in DB error for exercising handler error branches.
var errBoom = errors.New("boom")

// nowTime returns a fixed-ish timestamp for row fixtures.
func nowTime() time.Time { return time.Now().UTC() }

// strReader wraps a string body for httptest.NewRequest.
func strReader(s string) *strings.Reader { return strings.NewReader(s) }

// anyArgs returns n pgxmock.AnyArg() matchers, for WithArgs calls where the
// exact parameter values are not under test (only the SQL shape and outcome).
func anyArgs(n int) []interface{} {
	args := make([]interface{}, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// newMockHandlers returns Handlers backed by a pgxmock pool, and the mock
// itself for setting expectations.
func newMockHandlers(t *testing.T) (*Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return NewHandlers(mock), mock
}

// reqWithChiParam builds an httptest.Request carrying a chi URL param (e.g. "id").
func reqWithChiParam(method, url, body, key, val string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, bytes.NewBufferString(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	return reqWithChiParam2(r, key, val)
}

// reqWithChiParam2 attaches a chi URL param to an already-built request.
func reqWithChiParam2(r *http.Request, key, val string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}
