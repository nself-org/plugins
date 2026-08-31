package main

// handlers_apikeys.go — /v1/api-keys: tenant API-key self-service.
//
// Purpose: the dashboard's API Keys screen — list (id, prefix, name, scope,
//   created/last-used; NEVER the secret), create (raw nsk_ key returned
//   exactly once), and revoke. All rows are scoped to the credential's
//   tenant; cross-tenant revocation is a silent 404 (saas.RevokeAPIKey).
// Outputs: {"api_keys":[...]} · 201 {"api_key":{...},"key":"nsk_..."} · 204.
// Constraints: revoked keys stay listed (revoked=true) so the UI can show
//   history; the shared saas layer stores only the SHA-256 hash.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// apiKeyDTO is the wire shape of one key (secret never included).
type apiKeyDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	Scope      string  `json:"scope"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	Revoked    bool    `json:"revoked"`
}

func toAPIKeyDTO(k saas.APIKey) apiKeyDTO {
	dto := apiKeyDTO{
		ID:        k.ID,
		Name:      k.Name,
		Prefix:    k.KeyPrefix,
		Scope:     string(k.Scope),
		CreatedAt: k.CreatedAt.UTC().Format(time.RFC3339),
		Revoked:   k.RevokedAt != nil,
	}
	if k.LastUsedAt != nil {
		s := k.LastUsedAt.UTC().Format(time.RFC3339)
		dto.LastUsedAt = &s
	}
	return dto
}

// handleListAPIKeys — GET /v1/api-keys.
func (g *gateway) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	keys, err := saas.ListAPIKeys(r.Context(), g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "api key lookup failed")
		return
	}
	out := make([]apiKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKeyDTO(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": out})
}

// handleCreateAPIKey — POST /v1/api-keys {"name","scope"?} → 201 with the
// raw key (shown exactly once — only the hash is stored).
func (g *gateway) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	var req struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "name is required")
		return
	}
	if len(req.Name) > 100 {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "name must be at most 100 characters")
		return
	}
	scope := saas.ScopeFull
	switch req.Scope {
	case "", string(saas.ScopeFull):
	case string(saas.ScopeRead):
		scope = saas.ScopeRead
	default:
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", `scope must be "read" or "full"`)
		return
	}

	rawKey, id, err := saas.CreateAPIKey(r.Context(), g.db, tenantID, req.Name, scope)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "api key create failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_key": apiKeyDTO{
			ID:        id,
			Name:      req.Name,
			Prefix:    saas.DisplayPrefix(rawKey),
			Scope:     string(scope),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		},
		"key": rawKey, // the ONLY time the secret is ever returned
	})
}

// handleDeleteAPIKey — DELETE /v1/api-keys/{id} (revoke; idempotent within
// the tenant, 404 for unknown or cross-tenant ids).
func (g *gateway) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	okRevoked, err := saas.RevokeAPIKey(r.Context(), g.db, tenantID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "api key revoke failed")
		return
	}
	if !okRevoked {
		writeErr(w, http.StatusNotFound, "not_found", "api key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
