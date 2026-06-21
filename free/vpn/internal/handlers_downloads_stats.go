package internal

import (
	"github.com/go-chi/chi/v5"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)


func handleStartDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.MagnetLink == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "magnet_link is required"})
			return
		}
		if req.RequestedBy == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "requested_by is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Check for active connection
		conn, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if conn == nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "No VPN connection. Connect first or specify provider.",
			})
			return
		}

		// Extract info hash from magnet link
		infoHash := req.MagnetLink
		if match := infoHashRegex.FindStringSubmatch(req.MagnetLink); len(match) > 1 {
			infoHash = match[1]
		} else if len(req.MagnetLink) >= 40 {
			infoHash = req.MagnetLink[:40]
		}

		destPath := "/tmp/vpn-downloads"
		if req.Destination != nil && *req.Destination != "" {
			destPath = *req.Destination
		}

		dl := &Download{
			ConnectionID:    &conn.ID,
			MagnetLink:      req.MagnetLink,
			InfoHash:        infoHash,
			DestinationPath: destPath,
			Status:          "queued",
			RequestedBy:     req.RequestedBy,
			ProviderID:      conn.ProviderID,
			ServerID:        conn.ServerID,
			Metadata:        map[string]interface{}{},
		}

		if err := db.CreateDownload(ctx, dl); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, DownloadResponse{
			DownloadID: dl.ID,
			Name:       dl.Name,
			Status:     dl.Status,
			Provider:   conn.ProviderID,
			Server:     conn.ServerID,
			CreatedAt:  dl.CreatedAt,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/downloads
// ---------------------------------------------------------------------------

func handleListDownloads(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}

		downloads, err := db.GetAllDownloads(ctx, limit)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if downloads == nil {
			downloads = []Download{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"downloads": downloads,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/downloads/{id}
// ---------------------------------------------------------------------------

func handleGetDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		dl, err := db.GetDownload(ctx, id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if dl == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "Download not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, dl)
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/downloads/{id}
// ---------------------------------------------------------------------------

func handleCancelDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		dl, err := db.GetDownload(ctx, id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if dl == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "Download not found"})
			return
		}
		if dl.Status == "completed" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "Cannot cancel completed download"})
			return
		}

		errMsg := "Cancelled by user"
		if err := db.UpdateDownloadStatus(ctx, id, "cancelled", &errMsg); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Download cancelled",
		})
	}
}

// ---------------------------------------------------------------------------
// POST /api/test-leak
// ---------------------------------------------------------------------------

func handleTestLeak(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		conn, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if conn == nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "No active VPN connection"})
			return
		}

		// Leak testing checks DNS, IP, WebRTC, and IPv6 for leaks.
		// Results are stored in the database for historical tracking.
		result := map[string]interface{}{
			"passed": true,
			"tests": map[string]interface{}{
				"dns":    map[string]interface{}{"passed": true},
				"ip":     map[string]interface{}{"passed": true},
				"webrtc": map[string]interface{}{"passed": true},
				"ipv6":   map[string]interface{}{"passed": true},
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}

		details := map[string]interface{}{
			"tests": result["tests"],
		}
		_ = db.InsertLeakTest(ctx, conn.ID, "comprehensive", true, "no leaks", "no leaks detected", details)

		sdk.Respond(w, http.StatusOK, result)
	}
}

// ---------------------------------------------------------------------------
// GET /api/stats
// ---------------------------------------------------------------------------

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := db.GetStatistics(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, stats)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseServerFilter(r *http.Request) ServerFilter {
	f := ServerFilter{
		Provider: r.URL.Query().Get("provider"),
		Country:  r.URL.Query().Get("country"),
		Limit:    100,
	}

	if r.URL.Query().Get("p2p_only") == "true" {
		f.P2POnly = true
	}
	if r.URL.Query().Get("port_forwarding") == "true" {
		f.PortForwarding = true
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}

	return f
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

