package internal

import (
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListTokens(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The TS source does not expose a list tokens endpoint, but plugin.json
		// implies GET /v1/tokens. Return a simple message for now.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"message": "Use POST /v1/tokens/validate to check token status",
		})
	}
}
