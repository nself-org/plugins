package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Calendar
// =========================================================================

func handleGetCalendar() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Calendar returns an empty list; matches the TS implementation
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"calendar": []interface{}{}})
	}
}

// =========================================================================
// Queue
// =========================================================================

func handleGetQueue(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		queue, err := db.GetQueue(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get queue: %w", err))
			return
		}
		if queue == nil {
			queue = []AcquisitionQueueItem{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"queue": queue})
	}
}

func handleAddToQueue(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddToQueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentType == "" || req.ContentName == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentType and contentName are required"))
			return
		}

		accountID := sourceAccountID(r)
		item, err := db.AddToQueue(accountID, req.ContentType, req.ContentName, req.Year, req.Season, req.Episode, "api")
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to add to queue: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"item": item})
	}
}

