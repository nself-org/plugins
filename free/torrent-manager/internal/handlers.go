package internal

import (
	"log"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts all torrent-manager API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg *Config) {
	h := &handler{db: db, cfg: cfg}

	// Initialize Transmission client if configured
	if cfg.DefaultClient == "transmission" {
		h.transmission = NewTransmissionClient(
			cfg.TransmissionHost,
			cfg.TransmissionPort,
			cfg.TransmissionUsername,
			cfg.TransmissionPassword,
		)
		if err := h.transmission.Connect(); err != nil {
			log.Printf("torrent-manager: warning: failed to connect to Transmission: %v", err)
		} else {
			log.Printf("torrent-manager: connected to Transmission at %s:%d", cfg.TransmissionHost, cfg.TransmissionPort)
		}
	}

	r.Route("/v1", func(r chi.Router) {
		// Clients
		r.Get("/clients", h.handleListClients)

		// Search
		r.Post("/search", h.handleSearch)
		r.Post("/search/best-match", h.handleBestMatch)
		r.Post("/magnet", h.handleFetchMagnet)
		r.Get("/search/cache", h.handleSearchCache)

		// Downloads
		r.Post("/downloads", h.handleAddDownload)
		r.Get("/downloads", h.handleListDownloads)
		r.Get("/downloads/{id}", h.handleGetDownload)
		r.Delete("/downloads/{id}", h.handleDeleteDownload)
		r.Post("/downloads/{id}/pause", h.handlePauseDownload)
		r.Post("/downloads/{id}/resume", h.handleResumeDownload)

		// Stats
		r.Get("/stats", h.handleStats)

		// Seeding
		r.Get("/seeding", h.handleListSeeding)
		r.Put("/seeding/{id}/policy", h.handleUpdateSeedingPolicy)
		r.Get("/seeding/{id}/policy", h.handleGetSeedingPolicy)

		// Sources
		r.Get("/sources", h.handleListSources)
	})
}

type handler struct {
	db           *DB
	cfg          *Config
	transmission *TransmissionClient
}

