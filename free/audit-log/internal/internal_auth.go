package internal

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// InternalSecretHeader is the one header that carries PLUGIN_INTERNAL_SECRET on inter-plugin calls (D-0198).
const InternalSecretHeader = "X-Internal-Token"

// legacySecretHeaders are the older names for the same check, still read so callers that have not moved to
// InternalSecretHeader keep working. Remove them once every caller sends the canonical name.
var legacySecretHeaders = []string{"X-Plugin-Secret", "X-Internal-Secret", "X-Internal-Key"}

// presentedSecret returns the secret a request carries: the canonical header, else the first legacy header set.
func presentedSecret(r *http.Request) string {
	if v := r.Header.Get(InternalSecretHeader); v != "" {
		return v
	}
	for _, h := range legacySecretHeaders {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	return ""
}

// hasInternalSecret reports whether the request carries a non-empty secret equal to want, compared in constant time.
func hasInternalSecret(r *http.Request, want string) bool {
	got := presentedSecret(r)
	if got == "" || want == "" {
		return false
	}
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}
