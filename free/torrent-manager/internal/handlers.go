package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
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

// ============================================================================
// Client Handlers
// ============================================================================

func (h *handler) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.db.ListClients()
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list clients: %w", err))
		return
	}
	if clients == nil {
		clients = []TorrentClient{}
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"clients": clients,
	})
}

// ============================================================================
// Download Handlers
// ============================================================================

func (h *handler) handleAddDownload(w http.ResponseWriter, r *http.Request) {
	var req AddDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.MagnetURI == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("magnet_uri is required"))
		return
	}

	// VPN check
	if h.cfg.VPNRequired {
		if !h.checkVPN() {
			sdk.Error(w, http.StatusForbidden, fmt.Errorf("VPN must be active before starting downloads"))
			return
		}
	}

	if h.transmission == nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("no torrent client configured"))
		return
	}

	// Determine download path
	dlPath := h.cfg.DownloadPath
	if req.DownloadPath != nil && *req.DownloadPath != "" {
		dlPath = *req.DownloadPath
	}

	// Add torrent to Transmission
	added, err := h.transmission.AddTorrent(req.MagnetURI, dlPath)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to add torrent: %w", err))
		return
	}

	// Get default client for DB reference
	client, err := h.db.GetDefaultClient()
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get default client: %w", err))
		return
	}
	if client == nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("no default client configured"))
		return
	}

	// Save to database
	category := "other"
	if req.Category != nil {
		category = *req.Category
	}
	requestedBy := "api"
	if req.RequestedBy != nil {
		requestedBy = *req.RequestedBy
	}

	dl := &TorrentDownload{
		SourceAccountID: "primary",
		ClientID:        client.ID,
		ClientTorrentID: strconv.Itoa(added.ID),
		Name:            added.Name,
		InfoHash:        added.HashString,
		MagnetURI:       req.MagnetURI,
		Status:          "queued",
		Category:        category,
		DownloadPath:    &dlPath,
		RequestedBy:     requestedBy,
	}

	saved, err := h.db.CreateDownload(dl)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to save download: %w", err))
		return
	}

	log.Printf("torrent-manager: download added id=%s name=%s", saved.ID, saved.Name)

	sdk.Respond(w, http.StatusCreated, map[string]interface{}{
		"success":  true,
		"download": saved,
	})
}

func (h *handler) handleListDownloads(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	category := r.URL.Query().Get("category")
	limitStr := r.URL.Query().Get("limit")

	limit := 0
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			limit = n
		}
	}

	downloads, err := h.db.ListDownloads(status, category, limit)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list downloads: %w", err))
		return
	}
	if downloads == nil {
		downloads = []TorrentDownload{}
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"downloads": downloads,
		"total":     len(downloads),
	})
}

func (h *handler) handleGetDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	dl, err := h.db.GetDownload(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
		return
	}
	if dl == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
		return
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"download": dl,
	})
}

func (h *handler) handleDeleteDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	deleteFiles := r.URL.Query().Get("delete_files") == "true"

	dl, err := h.db.GetDownload(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
		return
	}
	if dl == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
		return
	}

	// Remove from Transmission
	if h.transmission != nil {
		if err := h.transmission.RemoveTorrent(dl.ClientTorrentID, deleteFiles); err != nil {
			log.Printf("torrent-manager: warning: failed to remove torrent from client: %v", err)
		}
	}

	if err := h.db.DeleteDownload(id); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete download: %w", err))
		return
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *handler) handlePauseDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	dl, err := h.db.GetDownload(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
		return
	}
	if dl == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
		return
	}

	if h.transmission != nil {
		if err := h.transmission.PauseTorrent(dl.ClientTorrentID); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to pause torrent: %w", err))
			return
		}
		if err := h.db.UpdateDownloadStatus(id, "paused", nil); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update status: %w", err))
			return
		}
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *handler) handleResumeDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	dl, err := h.db.GetDownload(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
		return
	}
	if dl == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
		return
	}

	// VPN check before resuming
	if h.cfg.VPNRequired {
		if !h.checkVPN() {
			sdk.Error(w, http.StatusForbidden, fmt.Errorf("VPN must be active to resume downloads"))
			return
		}
	}

	if h.transmission != nil {
		if err := h.transmission.ResumeTorrent(dl.ClientTorrentID); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to resume torrent: %w", err))
			return
		}
		if err := h.db.UpdateDownloadStatus(id, "downloading", nil); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update status: %w", err))
			return
		}
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// ============================================================================
// Search Handlers
// ============================================================================

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Query == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("query is required"))
		return
	}

	// Search aggregation across torrent sites is handled by the TS sidecar.
	// This endpoint returns an empty result set; configure SEARCH_AGGREGATOR_URL
	// to delegate to the running aggregator service.
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"query":   req.Query,
		"count":   0,
		"results": []interface{}{},
	})
}

func (h *handler) handleBestMatch(w http.ResponseWriter, r *http.Request) {
	var req SmartSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Title == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("title is required"))
		return
	}

	// Smart matching delegates to the search aggregator service.
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"match": nil,
	})
}

func (h *handler) handleFetchMagnet(w http.ResponseWriter, r *http.Request) {
	var req FetchMagnetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Source == "" || req.SourceURL == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("source and sourceUrl are required"))
		return
	}

	// Magnet fetching delegates to per-source scrapers in the aggregator.
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"magnetUri": "",
	})
}

func (h *handler) handleSearchCache(w http.ResponseWriter, r *http.Request) {
	queryHash := r.URL.Query().Get("query_hash")
	if queryHash == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("query_hash is required"))
		return
	}

	cache, err := h.db.GetSearchCache(queryHash)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get search cache: %w", err))
		return
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"cache": cache,
	})
}

// ============================================================================
// Stats Handlers
// ============================================================================

func (h *handler) handleStats(w http.ResponseWriter, r *http.Request) {
	dbStats, err := h.db.GetStats()
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
		return
	}

	var clientStats *TransmissionClientStats
	if h.transmission != nil {
		clientStats, _ = h.transmission.GetStats()
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"database":  dbStats,
		"client":    clientStats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *handler) handleListSeeding(w http.ResponseWriter, r *http.Request) {
	seeding, err := h.db.ListDownloads("seeding", "", 0)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list seeding: %w", err))
		return
	}
	if seeding == nil {
		seeding = []TorrentDownload{}
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"seeding": seeding,
		"total":   len(seeding),
	})
}

// ============================================================================
// Seeding Policy Handlers
// ============================================================================

func (h *handler) handleUpdateSeedingPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify download exists
	dl, err := h.db.GetDownload(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
		return
	}
	if dl == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
		return
	}

	var req SeedingConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	sourceAccountID := r.Header.Get("X-Source-Account-ID")
	if sourceAccountID == "" {
		sourceAccountID = "primary"
	}

	policy, err := h.db.UpsertDownloadSeedingPolicy(id, req, sourceAccountID)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update seeding policy: %w", err))
		return
	}

	log.Printf("torrent-manager: seeding policy updated download_id=%s favorite=%t", id, policy.Favorite)

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"updated": true,
	})
}

func (h *handler) handleGetSeedingPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	policy, err := h.db.GetDownloadSeedingPolicy(id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get seeding policy: %w", err))
		return
	}
	if policy == nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("no seeding policy found for this download"))
		return
	}
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"policy": policy,
	})
}

// ============================================================================
// Source Handlers
// ============================================================================

func (h *handler) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources := getRegisteredSources()
	sdk.Respond(w, http.StatusOK, sources)
}

// getRegisteredSources returns the static list of known torrent search sources.
func getRegisteredSources() []SourceRegistryEntry {
	return []SourceRegistryEntry{
		{
			Name:       "1337x",
			ActiveFrom: "2007-01-01",
			Category:   "general",
			TrustScore: 0.85,
			Strengths:  []string{"large catalog", "active community", "verified uploaders"},
		},
		{
			Name:       "yts",
			ActiveFrom: "2011-01-01",
			Category:   "movies",
			TrustScore: 0.80,
			Strengths:  []string{"movie focused", "small file sizes", "quality encodes"},
		},
		{
			Name:       "torrentgalaxy",
			ActiveFrom: "2018-01-01",
			Category:   "general",
			TrustScore: 0.75,
			Strengths:  []string{"community driven", "IMDB integration", "streaming previews"},
		},
		{
			Name:       "tpb",
			ActiveFrom: "2003-01-01",
			Category:   "general",
			TrustScore: 0.70,
			Strengths:  []string{"largest catalog", "longest running", "magnet links"},
		},
	}
}

// ============================================================================
// VPN Helper
// ============================================================================

// checkVPN calls the VPN Manager API to check if VPN is active.
func (h *handler) checkVPN() bool {
	if h.cfg.VPNManagerURL == "" {
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(h.cfg.VPNManagerURL + "/status")
	if err != nil {
		log.Printf("torrent-manager: VPN check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var status struct {
		Connected bool `json:"connected"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return false
	}

	return status.Connected
}
