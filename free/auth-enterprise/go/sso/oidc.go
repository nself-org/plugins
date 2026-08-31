// Package sso — oidc.go
//
// OIDC (OpenID Connect) identity provider integration.
//
// Flow:
//  1. nSelf builds an authorization URL for the configured IdP.
//  2. User authenticates at the IdP.
//  3. IdP redirects to /auth/sso/oidc/callback with a code.
//  4. nSelf exchanges the code for an ID token via the token endpoint.
//  5. ID token claims are mapped to nSelf user attributes.
//  6. nSelf issues a signed JWT; just-in-time provisioning creates the user
//     row if it does not yet exist.
//
// Supported IdPs with pre-built attribute maps:
//   - Google Workspace (discovery: accounts.google.com)
//   - Okta (discovery: <org>.okta.com)
//   - Microsoft Entra ID / Azure AD
//   - Generic OIDC (any RFC 8414 discovery document)
//
// SSO endpoints are ɳSelf+ only. Callers must check IsEnabled() and return
// 403 when NSELF_SSO != "true".
package sso

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared/httpclient"
)

// oidcClient is the package-level HTTP client used for OIDC discovery,
// token exchange, and userinfo calls. 30s budget per nSelf SSO scope.
var oidcClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// OIDCProvider holds the configuration for a single OIDC IdP.
type OIDCProvider struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	// IDPName is a human label: google|okta|entra|custom.
	IDPName      string            `json:"idp_name"`
	ClientID     string            `json:"client_id"`
	// ClientSecret is never serialised to JSON.
	ClientSecret string            `json:"-"`
	// DiscoveryURL is the base issuer URL; /.well-known/openid-configuration is appended.
	DiscoveryURL string            `json:"discovery_url"`
	// AttributeMap maps nSelf canonical attribute names (email, name, sub, role)
	// to the IdP-specific claim key to read.
	AttributeMap map[string]string `json:"attribute_map"`
	// RedirectURL is the nSelf OIDC callback endpoint.
	RedirectURL  string            `json:"redirect_url"`
	// Scopes to request (default: openid email profile).
	Scopes       []string          `json:"scopes"`
	// metadata is populated by Discover().
	metadata     *oidcMetadata
}

type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// OIDCClaims is the extracted set of claims from the IdP.
type OIDCClaims struct {
	Subject string
	Email   string
	Name    string
	Role    string
	Raw     map[string]interface{}
}

// Discover fetches and caches the OIDC well-known discovery document.
func (p *OIDCProvider) Discover(ctx context.Context) error {
	if p.metadata != nil {
		return nil
	}
	docURL := strings.TrimSuffix(p.DiscoveryURL, "/") + "/.well-known/openid-configuration"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return fmt.Errorf("build discovery request: %w", err)
	}
	resp, err := oidcClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc discovery returned HTTP %d for %s", resp.StatusCode, docURL)
	}
	var meta oidcMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return fmt.Errorf("decode oidc discovery: %w", err)
	}
	p.metadata = &meta
	return nil
}

// AuthorizationURL builds the IdP authorization endpoint URL. state is an
// opaque CSRF-prevention value that must be verified on callback.
func (p *OIDCProvider) AuthorizationURL(ctx context.Context, state string) (string, error) {
	if err := p.Discover(ctx); err != nil {
		return "", err
	}
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {p.ClientID},
		"redirect_uri":  {p.RedirectURL},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
	}
	return p.metadata.AuthorizationEndpoint + "?" + params.Encode(), nil
}

// ExchangeCode exchanges an authorization code for claims via the token endpoint
// (and optionally the userinfo endpoint).
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*OIDCClaims, error) {
	if err := p.Discover(ctx); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// POST to token endpoint.
	body := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {p.RedirectURL},
		"client_id":    {p.ClientID},
		"client_secret": {p.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.metadata.TokenEndpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := oidcClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc token request: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oidc token error %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Prefer userinfo endpoint for up-to-date claims when available.
	if p.metadata.UserinfoEndpoint != "" && tokenResp.AccessToken != "" {
		return p.fetchUserinfo(ctx, tokenResp.AccessToken)
	}
	if tokenResp.IDToken == "" {
		return nil, fmt.Errorf("oidc: neither userinfo endpoint usable nor id_token returned")
	}
	return p.parseIDTokenClaims(tokenResp.IDToken)
}

func (p *OIDCProvider) fetchUserinfo(ctx context.Context, accessToken string) (*OIDCClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.metadata.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := oidcClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc userinfo: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}
	return p.mapClaims(claims), nil
}

// parseIDTokenClaims decodes the payload section of a JWT without verifying the
// signature. Signature verification must be performed by the caller (e.g. via a
// separate JWKS fetch) before this function is invoked.
func (p *OIDCProvider) parseIDTokenClaims(idToken string) (*OIDCClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed id_token: expected 3 segments")
	}
	// Pad to a multiple of 4 for StdEncoding.
	seg := parts[1]
	if n := len(seg) % 4; n != 0 {
		seg += strings.Repeat("=", 4-n)
	}
	// Replace URL-safe chars before decoding.
	seg = strings.ReplaceAll(seg, "-", "+")
	seg = strings.ReplaceAll(seg, "_", "/")
	decoded, err := base64.StdEncoding.DecodeString(seg)
	if err != nil {
		return nil, fmt.Errorf("base64 decode id_token payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("parse id_token claims: %w", err)
	}
	return p.mapClaims(claims), nil
}

// mapClaims applies AttributeMap to translate IdP claim names to nSelf attributes.
func (p *OIDCProvider) mapClaims(raw map[string]interface{}) *OIDCClaims {
	get := func(canonicalKey string) string {
		rawKey := canonicalKey
		if mapped, ok := p.AttributeMap[canonicalKey]; ok {
			rawKey = mapped
		}
		if v, ok := raw[rawKey]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	return &OIDCClaims{
		Subject: get("sub"),
		Email:   get("email"),
		Name:    get("name"),
		Role:    get("role"),
		Raw:     raw,
	}
}

// ─── SSO feature flag ─────────────────────────────────────────────────────────

// IsEnabled returns true when NSELF_SSO=true (ɳSelf+ tier gate).
// All SSO handlers must return 403 when this returns false.
func IsEnabled() bool {
	return os.Getenv("NSELF_SSO") == "true"
}

// ─── Pre-built IdP factories ──────────────────────────────────────────────────

// GoogleWorkspaceOIDC returns a pre-configured OIDCProvider for Google Workspace.
func GoogleWorkspaceOIDC(tenantID, clientID, clientSecret, redirectURL string) *OIDCProvider {
	return &OIDCProvider{
		TenantID:     tenantID,
		IDPName:      "google",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		DiscoveryURL: "https://accounts.google.com",
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		AttributeMap: map[string]string{"sub": "sub", "email": "email", "name": "name"},
	}
}

// OktaOIDC returns a pre-configured OIDCProvider for Okta.
// orgDomain is e.g. "dev-123456.okta.com".
func OktaOIDC(tenantID, orgDomain, clientID, clientSecret, redirectURL string) *OIDCProvider {
	return &OIDCProvider{
		TenantID:     tenantID,
		IDPName:      "okta",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		DiscoveryURL: "https://" + orgDomain,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile", "groups"},
		AttributeMap: map[string]string{
			"sub": "sub", "email": "email", "name": "name", "role": "groups",
		},
	}
}

// EntraIDOIDC returns a pre-configured OIDCProvider for Microsoft Entra ID (Azure AD).
// tenantOrg is the Azure AD tenant domain or GUID, e.g. "contoso.onmicrosoft.com".
func EntraIDOIDC(tenantID, tenantOrg, clientID, clientSecret, redirectURL string) *OIDCProvider {
	return &OIDCProvider{
		TenantID:     tenantID,
		IDPName:      "entra",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		DiscoveryURL: "https://login.microsoftonline.com/" + tenantOrg + "/v2.0",
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		AttributeMap: map[string]string{
			"sub": "oid", "email": "preferred_username", "name": "name", "role": "roles",
		},
	}
}
