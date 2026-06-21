package internal

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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
