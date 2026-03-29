package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all feature-flags API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB) {
	eval := NewEvaluator(db)

	r.Route("/v1", func(r chi.Router) {
		// Flag CRUD
		r.Post("/flags", handleCreateFlag(db))
		r.Get("/flags", handleListFlags(db))
		r.Get("/flags/{key}", handleGetFlag(db))
		r.Put("/flags/{key}", handleUpdateFlag(db))
		r.Delete("/flags/{key}", handleDeleteFlag(db))

		// Evaluation
		r.Post("/evaluate", handleEvaluate(eval))
		r.Post("/evaluate/batch", handleEvaluateBatch(eval))
	})
}

// --- Flag handlers ---

func handleCreateFlag(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateFlagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Key == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("key is required"))
			return
		}

		flag, err := db.CreateFlag(req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create flag: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, flag)
	}
}

func handleListFlags(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flags, err := db.ListFlags()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list flags: %w", err))
			return
		}
		if flags == nil {
			flags = []Flag{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"flags": flags,
			"count": len(flags),
		})
	}
}

func handleGetFlag(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		flag, err := db.GetFlag(key)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get flag: %w", err))
			return
		}
		if flag == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, flag)
	}
}

func handleUpdateFlag(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		var req UpdateFlagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		flag, err := db.UpdateFlag(key, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update flag: %w", err))
			return
		}
		if flag == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, flag)
	}
}

func handleDeleteFlag(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		deleted, err := db.DeleteFlag(key)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete flag: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
		})
	}
}

// --- Evaluation handlers ---

func handleEvaluate(eval *Evaluator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req EvaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.FlagKey == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("flag_key is required"))
			return
		}
		if req.Context == nil {
			req.Context = map[string]interface{}{}
		}

		result := eval.Evaluate(req.FlagKey, req.UserID, req.Context)
		sdk.Respond(w, http.StatusOK, result)
	}
}

func handleEvaluateBatch(eval *Evaluator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BatchEvaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if len(req.FlagKeys) == 0 {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("flag_keys is required and must be non-empty"))
			return
		}
		if req.Context == nil {
			req.Context = map[string]interface{}{}
		}

		results := eval.EvaluateBatch(req.FlagKeys, req.UserID, req.Context)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
	}
}
