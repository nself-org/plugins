package internal

import (
	"time"

	"github.com/go-chi/chi/v5"
)

var startTime = time.Now()

// RegisterRoutes mounts all mDNS API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB) {
	// Operational endpoints
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive(db))

	// Service CRUD
	r.Route("/api", func(r chi.Router) {
		r.Post("/services", handleCreateService(db))
		r.Get("/services", handleListServices(db))
		r.Get("/services/{id}", handleGetService(db))
		r.Put("/services/{id}", handleUpdateService(db))
		r.Delete("/services/{id}", handleDeleteService(db))
		r.Post("/services/{id}/advertise", handleAdvertise(db))
		r.Post("/services/{id}/stop", handleStopAdvertise(db))

		// Discovery
		r.Post("/discover", handleDiscover(db))
		r.Get("/discovered", handleListDiscovered(db))

		// Stats
		r.Get("/stats", handleStats(db))
	})
}

// --- Operational handlers ---
