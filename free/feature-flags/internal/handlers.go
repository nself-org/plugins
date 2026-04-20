package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all feature-flags API routes on the given router.
// pubsub is optional: pass nil to disable Redis broadcast (falls back to TTL only).
func RegisterRoutes(r chi.Router, db *DB, pubsub *PubSub) {
	eval := NewEvaluator(db)

	r.Route("/v1", func(r chi.Router) {
		// Flag CRUD
		r.Post("/flags", handleCreateFlag(db, pubsub))
		r.Get("/flags", handleListFlags(db))
		r.Get("/flags/{key}", handleGetFlag(db))
		r.Put("/flags/{key}", handleUpdateFlag(db, pubsub))
		r.Delete("/flags/{key}", handleDeleteFlag(db, pubsub))

		// Convenience mutations with audit + pubsub
		r.Post("/flags/{key}/enable", handleEnableFlag(db, pubsub))
		r.Post("/flags/{key}/disable", handleDisableFlag(db, pubsub))
		r.Post("/flags/{key}/kill", handleKillFlag(db, pubsub))

		// Audit
		r.Get("/flags/{key}/history", handleFlagHistory(db))
		r.Get("/audit", handleAuditAll(db))

		// Prune (stale flags)
		r.Get("/flags/prune", handlePruneStale(db))

		// Evaluation
		r.Post("/evaluate", handleEvaluate(eval))
		r.Post("/evaluate/batch", handleEvaluateBatch(eval))
	})
}

// actorFromRequest extracts the actor (user_id or service token) from the request.
func actorFromRequest(r *http.Request) string {
	if ua := r.Header.Get("X-Actor"); ua != "" {
		return ua
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return "cli"
}

// broadcastInvalidation fires a Redis pub/sub broadcast (no-op if pubsub is nil).
func broadcastInvalidation(pubsub *PubSub, key string) {
	if pubsub == nil {
		return
	}
	pubsub.Broadcast(context.Background(), key)
}

// --- Flag handlers ---

func handleCreateFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
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

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), flag.Key, actor, "create",
			json.RawMessage("null"), marshalFlag(flag), nil)

		sdk.Respond(w, http.StatusCreated, flag)
	}
}

func handleListFlags(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flagType := r.URL.Query().Get("type")
		flags, err := db.ListFlags()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list flags: %w", err))
			return
		}
		if flags == nil {
			flags = []Flag{}
		}
		// Apply type filter if requested
		if flagType != "" {
			filtered := flags[:0]
			for _, f := range flags {
				if f.Type == flagType {
					filtered = append(filtered, f)
				}
			}
			flags = filtered
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

func handleUpdateFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		before, _ := db.GetFlag(key)

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

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), key, actor, "set",
			marshalFlag(before), marshalFlag(flag), nil)

		if req.Enabled != nil && !*req.Enabled {
			broadcastInvalidation(pubsub, key)
		}

		sdk.Respond(w, http.StatusOK, flag)
	}
}

func handleDeleteFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		before, _ := db.GetFlag(key)

		deleted, err := db.DeleteFlag(key)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete flag: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), key, actor, "delete",
			marshalFlag(before), json.RawMessage("null"), nil)

		broadcastInvalidation(pubsub, key)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"success": true})
	}
}

// handleEnableFlag sets enabled=true with audit + pubsub.
func handleEnableFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		before, _ := db.GetFlag(key)

		enabled := true
		flag, err := db.UpdateFlag(key, UpdateFlagRequest{Enabled: &enabled})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("enable flag: %w", err))
			return
		}
		if flag == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), key, actor, "enable",
			marshalFlag(before), marshalFlag(flag), nil)

		sdk.Respond(w, http.StatusOK, flag)
	}
}

// handleDisableFlag sets enabled=false with audit + pubsub broadcast.
func handleDisableFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		before, _ := db.GetFlag(key)

		enabled := false
		flag, err := db.UpdateFlag(key, UpdateFlagRequest{Enabled: &enabled})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("disable flag: %w", err))
			return
		}
		if flag == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), key, actor, "disable",
			marshalFlag(before), marshalFlag(flag), nil)

		broadcastInvalidation(pubsub, key)
		sdk.Respond(w, http.StatusOK, flag)
	}
}

// handleKillFlag performs an emergency kill-switch. reason is required.
// Broadcasts pubsub invalidation immediately for <5s SDK cache propagation.
func handleKillFlag(db *DB, pubsub *PubSub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")

		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if strings.TrimSpace(req.Reason) == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("reason is required for kill"))
			return
		}

		before, _ := db.GetFlag(key)

		enabled := false
		flag, err := db.UpdateFlag(key, UpdateFlagRequest{Enabled: &enabled})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("kill flag: %w", err))
			return
		}
		if flag == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("flag not found"))
			return
		}

		actor := actorFromRequest(r)
		_ = db.WriteAudit(r.Context(), key, actor, "kill",
			marshalFlag(before), marshalFlag(flag), &req.Reason)

		// Broadcast immediately — this is the kill-switch path
		broadcastInvalidation(pubsub, key)
		sdk.Respond(w, http.StatusOK, flag)
	}
}

// handleFlagHistory returns the audit log for a single flag.
func handleFlagHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		entries, err := db.ListAudit(r.Context(), key, 50)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("history: %w", err))
			return
		}
		if entries == nil {
			entries = []AuditEntry{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"entries": entries,
			"count":   len(entries),
		})
	}
}

// handleAuditAll returns paginated audit log across all flags.
func handleAuditAll(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For now, query all flags and aggregate. A dedicated all-audit query
		// can be added as a DB method when pagination is needed.
		entries, err := db.ListAudit(r.Context(), "", 100)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("audit: %w", err))
			return
		}
		if entries == nil {
			entries = []AuditEntry{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"entries": entries,
			"count":   len(entries),
		})
	}
}

// handlePruneStale lists flags that have exceeded their stale_after_days threshold.
// Returns the list; actual deletion is a separate administrative step.
func handlePruneStale(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dryRun := r.URL.Query().Get("dry_run") == "true"
		stale, err := db.ListStaleFlags(r.Context())
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("prune: %w", err))
			return
		}
		if stale == nil {
			stale = []Flag{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"stale":    stale,
			"count":    len(stale),
			"dry_run":  dryRun,
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
