package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

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
