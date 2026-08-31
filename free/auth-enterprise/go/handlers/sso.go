// Package handlers — sso.go
//
// HTTP handlers for SSO (SAML + OIDC) provider management and login flows.
//
// All SSO endpoints return 403 when NSELF_SSO != "true" (ɳSelf+ gate).
//
// Endpoint map:
//   GET    /auth/sso/providers            — list configured IdPs for tenant
//   POST   /auth/sso/providers            — add new IdP configuration
//   PATCH  /auth/sso/providers/:id        — update IdP configuration
//   DELETE /auth/sso/providers/:id        — remove IdP configuration
//   POST   /auth/sso/providers/:id/test   — validate IdP config (dry-run)
//   GET    /auth/sso/metadata             — SP SAML metadata XML
//   GET    /auth/sso/:provider/begin      — initiate SSO login
//   POST   /auth/sso/saml/callback        — SAML ACS endpoint
//   GET    /auth/sso/oidc/callback        — OIDC redirect callback
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-auth-enterprise/go/sso"
)

// SSOHandlers groups all SSO HTTP handlers.
type SSOHandlers struct {
	db            *pgxpool.Pool
	jit           *sso.JITProvisioner
	spEntityID    string
	samlACSURL    string
	oidcCallbackURL string
}

// NewSSOHandlers constructs an SSOHandlers.
func NewSSOHandlers(db *pgxpool.Pool, spEntityID, samlACSURL, oidcCallbackURL string) *SSOHandlers {
	return &SSOHandlers{
		db:              db,
		jit:             sso.NewJITProvisioner(db),
		spEntityID:      spEntityID,
		samlACSURL:      samlACSURL,
		oidcCallbackURL: oidcCallbackURL,
	}
}

// ssoGate returns true and writes 403 when SSO is not enabled.
func ssoGate(w http.ResponseWriter) bool {
	if !sso.IsEnabled() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintln(w, `{"error":"sso_not_enabled","message":"SSO requires a ɳSelf+ license. Set NSELF_SSO=true."}`)
		return true
	}
	return false
}

// ─── Provider management ──────────────────────────────────────────────────────

// ListProviders returns all enabled SSO providers for the caller's tenant.
func (h *SSOHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	tenantID := tenantIDFromContext(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := h.db.Query(ctx,
		`SELECT id, protocol, idp_name, enabled, created_at
		   FROM np_sso_providers WHERE tenant_id = $1 ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type row struct {
		ID        string    `json:"id"`
		Protocol  string    `json:"protocol"`
		IDPName   string    `json:"idp_name"`
		Enabled   bool      `json:"enabled"`
		CreatedAt time.Time `json:"created_at"`
	}
	var providers []row
	for rows.Next() {
		var p row
		if err := rows.Scan(&p.ID, &p.Protocol, &p.IDPName, &p.Enabled, &p.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		providers = append(providers, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"providers": providers,
	})
}

// CreateProvider stores a new OIDC or SAML provider configuration.
func (h *SSOHandlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	tenantID := tenantIDFromContext(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id required")
		return
	}

	var input sso.ProviderRecord
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if input.Protocol != "saml" && input.Protocol != "oidc" {
		writeError(w, http.StatusBadRequest, "protocol must be saml or oidc")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var id string
	err := h.db.QueryRow(ctx,
		`INSERT INTO np_sso_providers
		        (tenant_id, protocol, idp_name, metadata, attribute_map, enabled)
		 VALUES ($1, $2, $3, $4, $5, true)
		 RETURNING id`,
		tenantID, input.Protocol, input.IDPName,
		input.Metadata, input.AttributeMap,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "tenant_id": tenantID})
}

// DeleteProvider removes an SSO provider configuration.
func (h *SSOHandlers) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	tenantID := tenantIDFromContext(r)
	providerID := providerIDFromPath(r)
	if tenantID == "" || providerID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and provider_id required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := h.db.Exec(ctx,
		`DELETE FROM np_sso_providers WHERE id = $1 AND tenant_id = $2`,
		providerID, tenantID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": providerID})
}

// ─── SP SAML metadata ──────────────────────────────────────────────────────────

// SPMetadata returns the SP SAML metadata XML. IdPs configure nSelf as a trusted
// SP using this document.
func (h *SSOHandlers) SPMetadata(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	sp := &sso.SAMLProvider{
		SPEntityID: h.spEntityID,
		ACSURL:     h.samlACSURL,
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, sp.SPMetadataXML())
}

// ─── OIDC flow ─────────────────────────────────────────────────────────────────

// OIDCBegin initiates an OIDC login for the named IdP. Stores a CSRF state
// token in the session / short-lived DB row.
func (h *SSOHandlers) OIDCBegin(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	tenantID := tenantIDFromContext(r)
	providerID := providerIDFromPath(r)
	if tenantID == "" || providerID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id and provider_id required")
		return
	}

	provider, err := h.loadOIDCProvider(r.Context(), providerID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "provider not found: "+err.Error())
		return
	}

	state, err := generateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate state: "+err.Error())
		return
	}

	// Store state → providerID mapping for callback verification.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, _ = h.db.Exec(ctx,
		`INSERT INTO np_sso_state_cache (state, provider_id, tenant_id, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '10 minutes')`,
		state, providerID, tenantID,
	)

	authURL, err := provider.AuthorizationURL(r.Context(), state)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "build auth url: "+err.Error())
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// OIDCCallback handles the IdP redirect with an authorization code.
func (h *SSOHandlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	// Verify state and retrieve providerID + tenantID.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var providerID, tenantID string
	err := h.db.QueryRow(ctx,
		`DELETE FROM np_sso_state_cache
		  WHERE state = $1 AND expires_at > NOW()
		  RETURNING provider_id, tenant_id`,
		state,
	).Scan(&providerID, &tenantID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	provider, err := h.loadOIDCProvider(r.Context(), providerID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load provider: "+err.Error())
		return
	}

	claims, err := provider.ExchangeCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadRequest, "oidc exchange: "+err.Error())
		return
	}

	// JIT provision the user.
	user, err := h.jit.Provision(r.Context(), tenantID, providerID,
		claims.Subject, claims.Email, claims.Name, claims.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "jit provision: "+err.Error())
		return
	}

	// In a real deploy the caller would issue a nSelf JWT here and redirect to
	// the app. We return JSON so the outer auth handler can issue the token.
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  user.UserID,
		"email":    user.Email,
		"is_new":   user.IsNew,
		"provider": providerID,
	})
}

// ─── SAML flow ─────────────────────────────────────────────────────────────────

// SAMLCallback is the Assertion Consumer Service endpoint.
// The IdP POSTs a SAMLResponse form field here after authenticating the user.
func (h *SSOHandlers) SAMLCallback(w http.ResponseWriter, r *http.Request) {
	if ssoGate(w) {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "parse form: "+err.Error())
		return
	}

	samlResponse := r.FormValue("SAMLResponse")
	relayState := r.FormValue("RelayState")
	if samlResponse == "" {
		writeError(w, http.StatusBadRequest, "missing SAMLResponse")
		return
	}

	// Resolve provider from RelayState (stored as state token → providerID).
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var providerID, tenantID string
	err := h.db.QueryRow(ctx,
		`DELETE FROM np_sso_state_cache
		  WHERE state = $1 AND expires_at > NOW()
		  RETURNING provider_id, tenant_id`,
		relayState,
	).Scan(&providerID, &tenantID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired relay state")
		return
	}

	provider, err := h.loadSAMLProvider(r.Context(), providerID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load saml provider: "+err.Error())
		return
	}

	claims, err := provider.ParseResponse(samlResponse)
	if err != nil {
		writeError(w, http.StatusBadRequest, "saml response invalid: "+err.Error())
		return
	}

	user, err := h.jit.Provision(r.Context(), tenantID, providerID,
		claims.NameID, claims.Email, claims.Name, claims.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "jit provision: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  user.UserID,
		"email":    user.Email,
		"is_new":   user.IsNew,
		"provider": providerID,
	})
}

// ─── DB loader helpers ────────────────────────────────────────────────────────

func (h *SSOHandlers) loadOIDCProvider(ctx context.Context, providerID, tenantID string) (*sso.OIDCProvider, error) {
	var rec sso.ProviderRecord
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := h.db.QueryRow(ctx,
		`SELECT id, tenant_id, protocol, idp_name, metadata, attribute_map, enabled
		   FROM np_sso_providers WHERE id = $1 AND tenant_id = $2 AND protocol = 'oidc'`,
		providerID, tenantID,
	).Scan(&rec.ID, &rec.TenantID, &rec.Protocol, &rec.IDPName,
		&rec.Metadata, &rec.AttributeMap, &rec.Enabled)
	if err != nil {
		return nil, fmt.Errorf("load oidc provider: %w", err)
	}

	// Decode the JSONB metadata blob into OIDCProvider fields.
	var meta struct {
		DiscoveryURL string   `json:"discovery_url"`
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURL  string   `json:"redirect_url"`
		Scopes       []string `json:"scopes"`
	}
	if err := json.Unmarshal(rec.Metadata, &meta); err != nil {
		return nil, fmt.Errorf("decode oidc metadata: %w", err)
	}

	var attrMap map[string]string
	_ = json.Unmarshal(rec.AttributeMap, &attrMap)

	return &sso.OIDCProvider{
		ID:            rec.ID,
		TenantID:      rec.TenantID,
		IDPName:       rec.IDPName,
		ClientID:      meta.ClientID,
		ClientSecret:  meta.ClientSecret,
		DiscoveryURL:  meta.DiscoveryURL,
		RedirectURL:   h.oidcCallbackURL,
		Scopes:        meta.Scopes,
		AttributeMap:  attrMap,
	}, nil
}

func (h *SSOHandlers) loadSAMLProvider(ctx context.Context, providerID, tenantID string) (*sso.SAMLProvider, error) {
	var rec sso.ProviderRecord
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := h.db.QueryRow(ctx,
		`SELECT id, tenant_id, protocol, idp_name, metadata, attribute_map, enabled
		   FROM np_sso_providers WHERE id = $1 AND tenant_id = $2 AND protocol = 'saml'`,
		providerID, tenantID,
	).Scan(&rec.ID, &rec.TenantID, &rec.Protocol, &rec.IDPName,
		&rec.Metadata, &rec.AttributeMap, &rec.Enabled)
	if err != nil {
		return nil, fmt.Errorf("load saml provider: %w", err)
	}

	var meta struct {
		SSOURL      string `json:"sso_url"`
		EntityID    string `json:"entity_id"`
		Certificate string `json:"certificate"`
	}
	if err := json.Unmarshal(rec.Metadata, &meta); err != nil {
		return nil, fmt.Errorf("decode saml metadata: %w", err)
	}

	var attrMap map[string]string
	_ = json.Unmarshal(rec.AttributeMap, &attrMap)

	return &sso.SAMLProvider{
		ID:           rec.ID,
		TenantID:     rec.TenantID,
		IDPName:      rec.IDPName,
		SSOURL:       meta.SSOURL,
		EntityID:     meta.EntityID,
		Certificate:  meta.Certificate,
		SPEntityID:   h.spEntityID,
		ACSURL:       h.samlACSURL,
		AttributeMap: attrMap,
	}, nil
}

// ─── Path helpers ─────────────────────────────────────────────────────────────

// providerIDFromPath extracts the :id path segment. Works with chi or manual
// query-param fallback.
func providerIDFromPath(r *http.Request) string {
	// chi path param.
	if id := r.PathValue("id"); id != "" {
		return id
	}
	// query param fallback for non-chi routers.
	return r.URL.Query().Get("provider_id")
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
