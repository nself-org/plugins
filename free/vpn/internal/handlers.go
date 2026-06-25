package internal

import (
	"os"

	"github.com/go-chi/chi/v5"
)

// encryptionKey is loaded from ENCRYPTION_KEY env var at init.
var encryptionKey string

func init() {
	encryptionKey = os.Getenv("ENCRYPTION_KEY")
}

// RegisterRoutes mounts all VPN plugin endpoints on the given router.
func RegisterRoutes(r chi.Router, db *DB) {
	r.Get("/api/health", handleAPIHealth(db))
	r.Get("/api/providers", handleListProviders(db))
	r.Get("/api/providers/{id}", handleGetProvider(db))
	r.Post("/api/providers/{id}/credentials", handleStoreCredentials(db))
	r.Get("/api/servers", handleListServers(db))
	r.Get("/api/servers/p2p", handleListP2PServers(db))
	r.Post("/api/servers/sync", handleSyncServers(db))
	r.Post("/api/connect", handleConnect(db))
	r.Post("/api/disconnect", handleDisconnect(db))
	r.Get("/api/status", handleStatus(db))
	r.Post("/api/download", handleStartDownload(db))
	r.Get("/api/downloads", handleListDownloads(db))
	r.Get("/api/downloads/{id}", handleGetDownload(db))
	r.Delete("/api/downloads/{id}", handleCancelDownload(db))
	r.Post("/api/test-leak", handleTestLeak(db))
	r.Get("/api/stats", handleStats(db))
}

// ---------------------------------------------------------------------------
// GET /api/health
// ---------------------------------------------------------------------------

