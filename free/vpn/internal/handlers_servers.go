package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListServers(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		f := parseServerFilter(r)
		servers, err := db.GetServers(ctx, f)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if servers == nil {
			servers = []Server{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"servers": servers,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/servers/p2p
// ---------------------------------------------------------------------------

func handleListP2PServers(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		f := parseServerFilter(r)
		f.P2POnly = true

		servers, err := db.GetServers(ctx, f)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if servers == nil {
			servers = []Server{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"servers": servers,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /api/servers/sync
// ---------------------------------------------------------------------------

func handleSyncServers(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ServerSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Provider == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
			return
		}

		// Server sync fetches the latest server list from the VPN provider.
		// The actual provider API calls are handled by CLI-side provider modules.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success":        true,
			"servers_synced": 0,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /api/connect
// ---------------------------------------------------------------------------

func handleConnect(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ConnectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Provider == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Check for existing active connection
		active, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if active != nil {
			sdk.Respond(w, http.StatusConflict, map[string]interface{}{
				"error":         "Already connected",
				"connection_id": active.ID,
				"provider":      active.ProviderID,
			})
			return
		}

		// Verify provider exists
		provider, err := db.GetProvider(ctx, req.Provider)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if provider == nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "Unknown provider"})
			return
		}

		// Verify credentials
		if encryptionKey != "" {
			hasCreds, _ := db.HasCredentials(ctx, req.Provider, encryptionKey)
			if !hasCreds {
				sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "No credentials found for provider"})
				return
			}
		}

		protocol := "wireguard"
		if req.Protocol != nil {
			protocol = *req.Protocol
		}
		killSwitch := true
		if req.KillSwitch != nil {
			killSwitch = *req.KillSwitch
		}
		now := time.Now().UTC()

		conn := &Connection{
			ProviderID:        req.Provider,
			Protocol:          protocol,
			Status:            "connected",
			KillSwitchEnabled: killSwitch,
			RequestedBy:       req.RequestedBy,
			ConnectedAt:       &now,
			DNSServers:        []string{},
			Metadata:          map[string]interface{}{},
		}

		if err := db.CreateConnection(ctx, conn); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		resp := ConnectResponse{
			ConnectionID: conn.ID,
			Provider:     conn.ProviderID,
			Server:       ptrStr(conn.ServerID),
			VPNIP:        ptrStr(conn.VPNIP),
			Interface:    ptrStr(conn.InterfaceName),
			DNSServers:   conn.DNSServers,
			PortForwarded: conn.PortForwarded,
			ConnectedAt:  conn.ConnectedAt,
		}

		sdk.Respond(w, http.StatusOK, resp)
	}
}

// ---------------------------------------------------------------------------
// POST /api/disconnect
// ---------------------------------------------------------------------------

func handleDisconnect(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		conn, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if conn == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "No active connection"})
			return
		}

		now := time.Now().UTC()
		var dur *int
		if conn.ConnectedAt != nil {
			d := int(now.Sub(*conn.ConnectedAt).Seconds())
			dur = &d
		}

		if err := db.UpdateConnectionStatus(ctx, conn.ID, "disconnected", &now, dur); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Disconnected",
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/status
// ---------------------------------------------------------------------------

func handleStatus(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		conn, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if conn == nil {
			sdk.Respond(w, http.StatusOK, StatusResponse{Connected: false})
			return
		}

		resp := StatusResponse{
			Connected:         true,
			ConnectionID:      &conn.ID,
			Provider:          &conn.ProviderID,
			VPNIP:             conn.VPNIP,
			Interface:         conn.InterfaceName,
			Protocol:          &conn.Protocol,
			KillSwitchEnabled: &conn.KillSwitchEnabled,
			PortForwarded:     conn.PortForwarded,
		}

		if conn.ConnectedAt != nil {
			uptime := int(time.Since(*conn.ConnectedAt).Seconds())
			resp.UptimeSeconds = &uptime
		}
		if conn.BytesSent > 0 {
			resp.BytesSent = &conn.BytesSent
		}
		if conn.BytesReceived > 0 {
			resp.BytesReceived = &conn.BytesReceived
		}

		sdk.Respond(w, http.StatusOK, resp)
	}
}

// ---------------------------------------------------------------------------
// POST /api/download
// ---------------------------------------------------------------------------

var infoHashRegex = regexp.MustCompile(`urn:btih:([a-fA-F0-9]{40})`)
