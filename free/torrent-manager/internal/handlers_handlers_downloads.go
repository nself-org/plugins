package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"strconv"
	sdk "github.com/nself-org/plugin-sdk"
)

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

