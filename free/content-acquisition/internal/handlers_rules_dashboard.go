package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Download Rules
// =========================================================================

func handleCreateRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" || req.Action == "" || req.Conditions == nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name, conditions, and action are required"))
			return
		}

		priority := 0
		if req.Priority != nil {
			priority = *req.Priority
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		accountID := sourceAccountID(r)
		rule, err := db.CreateDownloadRule(accountID, req.Name, req.Conditions, req.Action, priority, enabled)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create rule: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"rule": rule})
	}
}

func handleListRules(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		rules, err := db.ListDownloadRules(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list rules: %w", err))
			return
		}
		if rules == nil {
			rules = []DownloadRule{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"rules": rules})
	}
}

func handleUpdateRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		rule, err := db.UpdateDownloadRule(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update rule: %w", err))
			return
		}
		if rule == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"rule": rule})
	}
}

func handleDeleteRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteDownloadRule(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete rule: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

// Size-cap exception: single-responsibility HTTP route handler — 74L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func handleTestRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		rule, err := db.GetDownloadRule(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get rule: %w", err))
			return
		}
		if rule == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}

		var req TestRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		// Evaluate conditions against sample data
		var conditions map[string]interface{}
		if err := json.Unmarshal(rule.Conditions, &conditions); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("invalid rule conditions: %w", err))
			return
		}

		allMatch := true
		type fieldResult struct {
			Field    string      `json:"field"`
			Expected interface{} `json:"expected"`
			Actual   interface{} `json:"actual"`
			Match    bool        `json:"match"`
		}
		var results []fieldResult

		for field, expected := range conditions {
			actual := req.Sample[field]
			match := false

			switch ev := expected.(type) {
			case string:
				if av, ok := actual.(string); ok {
					match = strings.Contains(strings.ToLower(av), strings.ToLower(ev))
				}
			case float64:
				if av, ok := actual.(float64); ok {
					match = av >= ev
				}
			case bool:
				match = actual == expected
			default:
				match = actual == expected
			}

			results = append(results, fieldResult{
				Field:    field,
				Expected: expected,
				Actual:   actual,
				Match:    match,
			})
			if !match {
				allMatch = false
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
			"action":    rule.Action,
			"matches":   allMatch,
			"results":   results,
		})
	}
}

// =========================================================================
// Dashboard
// =========================================================================
