package internal

import (
	"fmt"
	"net/http"

	sdk "github.com/nself-org/plugin-sdk"
)

// ============================================================================
// Stats Handler
// ============================================================================

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		scopedDB := db.ForSourceAccount(sourceAccountID)

		stats, err := scopedDB.GetStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get stats: %w", err))
			return
		}

		sdk.Respond(w, http.StatusOK, stats)
	}
}

