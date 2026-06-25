package internal

import (
	"net/http"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"
)

// getSourceAccountID extracts the multi-app isolation account ID from an HTTP
// request. Delegates to sdk.SourceAccountID which accepts all four canonical
// header spellings (X-Source-Account-ID, X-Source-Account-Id,
// X-Hasura-Source-Account-Id, X-Source-Account). Returns "primary" when none
// are present. Fix: previously only checked X-Source-Account-ID (P4-E0 audit).
func getSourceAccountID(r *http.Request) string {
	return sdk.SourceAccountID(r)
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
