package internal

import (
	"fmt"
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

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
