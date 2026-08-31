// Purpose: shared test helpers for handler tests (chi URL params, error stubs).
// Inputs: n/a
// Outputs: helper funcs used across handlers_db_test.go
// Constraints: test-only, no production impact.
package internal

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
)

// assertErr builds a plain error for pgxmock WillReturnError expectations.
func assertErr(msg string) error {
	return errors.New(msg)
}

// withChiContext attaches a chi.RouteContext to the request context so
// chi.URLParam(r, "id") resolves in handler unit tests without a full router.
func withChiContext(r *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
}

// anyArgs returns n pgxmock.AnyArg() matchers, for WithArgs(anyArgs(n)...)
// calls where the exact bound values don't matter to the test.
func anyArgs(n int) []interface{} {
	args := make([]interface{}, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}
