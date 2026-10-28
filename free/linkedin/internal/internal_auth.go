package internal

import "net/http"

// legacySecretHeaders are the older names for the X-Internal-Token check (D-0198), still read so callers that have
// not moved to the canonical name keep working. Remove them once every caller sends X-Internal-Token.
var legacySecretHeaders = []string{"X-Plugin-Secret", "X-Internal-Secret", "X-Internal-Key"}

// presentedSecret returns the secret a request carries: X-Internal-Token, else the first legacy header set.
func presentedSecret(r *http.Request) string {
	if v := r.Header.Get("X-Internal-Token"); v != "" {
		return v
	}
	for _, h := range legacySecretHeaders {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	return ""
}
