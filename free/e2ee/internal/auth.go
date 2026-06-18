package internal

// auth.go — gateway-trust authentication layer (CR-C BLOCK fix, P3 2026-06-18).
//
// SECURITY MODEL (canonical nSelf plugin pattern — mirrors paid/auth):
//   This plugin sits BEHIND Hasura. Hasura Actions are wired with
//   `forward_client_headers: true` and forward the AUTHENTICATED principal as
//   request headers:
//     - X-Hasura-User-Id          — the verified user id (JWT `sub` claim)
//     - X-Hasura-Role             — the verified Hasura role
//     - X-Source-Account-ID /     — the multi-app isolation key (Convention A)
//       X-Hasura-Source-Account-Id / X-Source-Account
//   These headers are the trust boundary. The plugin TRUSTS them and enforces
//   ownership against them. Port 3055 MUST NOT be publicly routed — only the
//   gateway (Hasura/nginx) may reach it. As defense-in-depth, requests that
//   arrive with NO authenticated user id FAIL CLOSED (401).
//
// Inputs:    *http.Request (gateway-forwarded headers).
// Outputs:   the authenticated principal (user id + source account), or a 401.
// Constraints: never trust request-body user_id for authorization — only this
//   header-derived principal may be used as the ownership subject.

import (
	"context"
	"net/http"
	"strings"
)

// principalCtxKey carries the authenticated principal on the request context.
type principalCtxKey struct{}

// Principal is the authenticated caller derived from gateway-forwarded headers.
type Principal struct {
	UserID         string // X-Hasura-User-Id (verified JWT subject)
	SourceAccount  string // multi-app isolation key (Convention A)
	Role           string // X-Hasura-Role
}

// hasuraUserIDHeaders are the headers Hasura forwards for the verified subject.
var hasuraUserIDHeaders = []string{
	"X-Hasura-User-Id",
	"X-Hasura-User-ID",
}

// sourceAccountHeaders are the accepted spellings of the isolation-key header.
var sourceAccountHeaders = []string{
	"X-Source-Account-ID",
	"X-Source-Account-Id",
	"X-Hasura-Source-Account-Id",
	"X-Source-Account",
}

// firstHeader returns the first non-empty value among the named headers.
func firstHeader(r *http.Request, names []string) string {
	for _, n := range names {
		if v := strings.TrimSpace(r.Header.Get(n)); v != "" {
			return v
		}
	}
	return ""
}

// principalFrom extracts the authenticated principal from forwarded headers.
// The source account defaults to 'primary' (Convention A) when not supplied,
// but a missing user id is treated as unauthenticated by the middleware.
func principalFrom(r *http.Request) Principal {
	acct := firstHeader(r, sourceAccountHeaders)
	if acct == "" {
		acct = "primary"
	}
	return Principal{
		UserID:        firstHeader(r, hasuraUserIDHeaders),
		SourceAccount: acct,
		Role:          strings.TrimSpace(r.Header.Get("X-Hasura-Role")),
	}
}

// RequireAuth is HTTP middleware that extracts the gateway-forwarded principal
// and rejects (401) any request missing an authenticated user id. The gateway
// is the trust boundary; this middleware makes missing-header requests fail
// closed so the plugin can never be reached unauthenticated (defense in depth).
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r)
		if p.UserID == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		ctx := context.WithValue(r.Context(), principalCtxKey{}, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// principalOf returns the authenticated principal stored on ctx by RequireAuth.
// The ok flag is false when no principal is present (route not behind RequireAuth).
func principalOf(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}
