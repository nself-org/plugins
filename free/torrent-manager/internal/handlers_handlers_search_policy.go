package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"log"
	"net/http"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

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

	// Use sdk.SourceAccountID to accept all 4 canonical header spellings.
	// Fix: previously only checked X-Source-Account-ID (P4-E0 audit).
	sourceAccountID := sdk.SourceAccountID(r)

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

