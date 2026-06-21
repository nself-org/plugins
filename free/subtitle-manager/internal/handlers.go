package internal

import (

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts all subtitle-manager API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg *Config, osClient *OpenSubtitlesClient) {
	syncer := NewSynchronizer(cfg)
	qc := NewSubtitleQC()
	norm := NewNormalizer()

	r.Route("/v1", func(r chi.Router) {
		r.Get("/subtitles", handleListSubtitles(db))
		r.Get("/downloads", handleListDownloads(db))
		r.Get("/stats", handleGetStats(db))
		r.Post("/search", handleSearch(osClient))
		r.Post("/search/hash", handleSearchHash(osClient))
		r.Post("/download", handleDownload(db, cfg, osClient, qc))
		r.Post("/sync", handleSync(cfg, syncer))
		r.Post("/qc", handleQC(db, qc))
		r.Post("/normalize", handleNormalize(norm))
		r.Post("/fetch-best", handleFetchBest(db, cfg, osClient, syncer, norm))
		r.Delete("/downloads/{id}", handleDeleteDownload(db))
	})
}

// ---------------------------------------------------------------------------
// GET /v1/subtitles
// ---------------------------------------------------------------------------

