package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// History
// =========================================================================

func handleGetHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		days := 90
		if v := r.URL.Query().Get("days"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				days = d
			}
		}
		history, err := db.ListAcquisitionHistory(accountID, days)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list history: %w", err))
			return
		}
		if history == nil {
			history = []AcquisitionHistoryItem{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"history": history})
	}
}

// =========================================================================
// Quality Profiles
// =========================================================================

func handleListProfiles(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		profiles, err := db.ListProfiles(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list profiles: %w", err))
			return
		}
		if profiles == nil {
			profiles = []QualityProfile{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"profiles": profiles})
	}
}

func handleCreateProfile(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name is required"))
			return
		}

		qualities := req.PreferredQualities
		if len(qualities) == 0 {
			qualities = []string{"1080p", "720p"}
		}
		minSeeders := 1
		if req.MinSeeders != nil {
			minSeeders = *req.MinSeeders
		}

		accountID := sourceAccountID(r)
		profile, err := db.CreateQualityProfile(accountID, req.Name, qualities, minSeeders)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create profile: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"profile": profile})
	}
}

func handleGetPresets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"presets": QualityPresets})
	}
}

