package internal

import (
	"time"

	"github.com/go-chi/chi/v5"
)

var startedAt = time.Now()

// RegisterRoutes mounts all donorbox endpoints on the given router.
func RegisterRoutes(r chi.Router, db *DB, client *DonorboxClient, webhookSecret string) {
	// Health probes
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive())
	r.Get("/status", handleStatus(db))

	// Sync operations (require API client)
	r.Post("/sync", handleSync(db, client))
	r.Post("/reconcile", handleReconcile(db, client))

	// Webhook
	r.Post("/webhooks/donorbox", handleWebhook(db, webhookSecret))

	// API queries
	r.Get("/api/campaigns", handleListCampaigns(db))
	r.Get("/api/donors", handleListDonors(db))
	r.Get("/api/donations", handleListDonations(db))
	r.Get("/api/plans", handleListPlans(db))
	r.Get("/api/stats", handleGetStats(db))
	r.Get("/api/events", handleListWebhookEvents(db))
}

