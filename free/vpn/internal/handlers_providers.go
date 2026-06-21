package internal

import (
	"github.com/go-chi/chi/v5"
	"context"
	"encoding/json"
	"net/http"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleAPIHealth(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp := HealthResponse{}

		conn, err := db.GetActiveConnection(ctx)
		if err != nil {
			sdk.Respond(w, http.StatusOK, resp)
			return
		}

		if conn != nil {
			resp.VPNConnected = true
			lt, err := db.GetLatestLeakTest(ctx, conn.ID)
			if err == nil && lt != nil {
				ts := lt.TestedAt.Format(time.RFC3339)
				resp.LastTest = &ts
				// Parse details to check individual test results
				if tests, ok := lt.Details["tests"]; ok {
					if testsMap, ok := tests.(map[string]interface{}); ok {
						if dns, ok := testsMap["dns"].(map[string]interface{}); ok {
							if passed, ok := dns["passed"].(bool); ok {
								resp.DNSLeak = !passed
							}
						}
						if webrtc, ok := testsMap["webrtc"].(map[string]interface{}); ok {
							if passed, ok := webrtc["passed"].(bool); ok {
								resp.WebRTCLeak = !passed
							}
						}
						if ipv6, ok := testsMap["ipv6"].(map[string]interface{}); ok {
							if passed, ok := ipv6["passed"].(bool); ok {
								resp.IPv6Leak = !passed
							}
						}
					}
				}
			}
		}

		sdk.Respond(w, http.StatusOK, resp)
	}
}

// ---------------------------------------------------------------------------
// GET /api/providers
// ---------------------------------------------------------------------------

func handleListProviders(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		providers, err := db.GetAllProviders(ctx)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if providers == nil {
			providers = []Provider{}
		}

		type providerItem struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			CLIAvailable    bool   `json:"cli_available"`
			PortForwarding  bool   `json:"port_forwarding"`
			P2PServers      int    `json:"p2p_servers"`
			TotalServers    int    `json:"total_servers"`
		}

		items := make([]providerItem, len(providers))
		for i, p := range providers {
			items[i] = providerItem{
				ID:             p.ID,
				Name:           p.DisplayName,
				CLIAvailable:   p.CLIAvailable,
				PortForwarding: p.PortForwardingSupported,
				P2PServers:     p.P2PServerCount,
				TotalServers:   p.TotalServers,
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"providers": items,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/providers/{id}
// ---------------------------------------------------------------------------

func handleGetProvider(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "provider id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		provider, err := db.GetProvider(ctx, id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if provider == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "Provider not found"})
			return
		}

		hasCreds, _ := db.HasCredentials(ctx, provider.ID, encryptionKey)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"id":                        provider.ID,
			"name":                      provider.Name,
			"display_name":              provider.DisplayName,
			"cli_available":             provider.CLIAvailable,
			"api_available":             provider.APIAvailable,
			"port_forwarding_supported": provider.PortForwardingSupported,
			"p2p_all_servers":           provider.P2PAllServers,
			"p2p_server_count":          provider.P2PServerCount,
			"total_servers":             provider.TotalServers,
			"total_countries":           provider.TotalCountries,
			"wireguard_supported":       provider.WireguardSupported,
			"openvpn_supported":         provider.OpenVPNSupported,
			"kill_switch_available":     provider.KillSwitchAvailable,
			"has_credentials":           hasCreds,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /api/providers/{id}/credentials
// ---------------------------------------------------------------------------

func handleStoreCredentials(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "provider id is required"})
			return
		}
		if encryptionKey == "" {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "ENCRYPTION_KEY not configured"})
			return
		}

		var req CredentialRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		provider, err := db.GetProvider(ctx, id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if provider == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "Provider not found"})
			return
		}

		username := ptrStr(req.Username)
		password := ptrStr(req.Password)
		token := ptrStr(req.Token)
		accountNum := ptrStr(req.AccountNumber)
		apiKey := ptrStr(req.APIKey)

		if err := db.UpsertCredentials(ctx, id, username, password, token, accountNum, apiKey, encryptionKey); err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Credentials stored",
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/servers
// ---------------------------------------------------------------------------

