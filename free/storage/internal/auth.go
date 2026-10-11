package internal

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// secretHeader carries PLUGIN_INTERNAL_SECRET on every /storage/* request.
const secretHeader = "X-Plugin-Secret"

// secretEqual compares two secrets in constant time. Both sides are hashed first so the comparison takes the same
// time for any lengths (ConstantTimeCompare alone returns early on a length mismatch).
func secretEqual(got, want string) bool {
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

// requireInternalSecret gates every /storage/* route behind PLUGIN_INTERNAL_SECRET (D-0396).
//
// The account header (X-Hasura-Source-Account-Id) picks the tenant, so it is trusted only on requests that carry the
// secret: callers are trusted backend services, never end users. Without this gate any client able to reach the
// port (the generated nginx routes pass client headers through) could name any account.
//
// Secret unset: 503, fail closed (the plugin still boots and /health stays up). Secret missing or wrong: 401.
// It must run before anything reads the account header.
func (s *Server) requireInternalSecret(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := ""
		if s.cfg != nil {
			want = s.cfg.InternalSecret
		}
		if want == "" {
			http.Error(w, `{"error":"PLUGIN_INTERNAL_SECRET not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if !secretEqual(r.Header.Get(secretHeader), want) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
