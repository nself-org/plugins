package main

// proxy.go — loopback HTTP calls to the nSentry plugins.
//
// Purpose: thin request helper for the gateway→plugin hop. Credential rules:
//   - nsk_ API key → forwarded verbatim (plugins authenticate it via
//     shared/saas themselves).
//   - dashboard session JWT → NEVER forwarded. The gateway is the session
//     authority (SAAS_JWT_HS256_SECRET); plugins verify with a DIFFERENT
//     secret (HASURA_GRAPHQL_JWT_SECRET) and would 401 it. Instead the
//     gateway mints a short-lived SERVICE JWT signed with the upstream
//     secret (cfg.UpstreamJWTSecret) carrying the resolved tenant in Hasura
//     claims — this satisfies plugins with their own strict JWT middleware
//     (e.g. nself-incident-mgmt) without sharing the session secret.
//   - the resolved tenant is ALWAYS injected as X-Hasura-Tenant-Id (the
//     internal-only header nginx strips from external requests).
// Inputs:  upstream base URL + path, original request (for auth headers),
//   optional JSON body.
// Outputs: status code + raw body bytes.
// Constraints: 10s timeout; loopback only in the single-box topology —
//   never exposed as an open proxy (paths are fixed by the handlers).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// upstreamClient is shared across all plugin calls.
var upstreamClient = &http.Client{Timeout: 10 * time.Second}

// serviceJWTTTL bounds the gateway→plugin service token lifetime.
const serviceJWTTTL = 2 * time.Minute

// mintServiceJWT signs a short-lived upstream token for the resolved tenant.
func (g *gateway) mintServiceJWT(tenantID string) (string, error) {
	now := time.Now().UTC()
	return saas.SignHS256(map[string]any{
		"iss":       "nself-saas-gateway",
		"sub":       tenantID,
		"tenant_id": tenantID,
		"iat":       now.Unix(),
		"exp":       now.Add(serviceJWTTTL).Unix(),
		"https://hasura.io/jwt/claims": map[string]any{
			"x-hasura-tenant-id":     tenantID,
			"x-hasura-user-id":       tenantID,
			"x-hasura-default-role":  "user",
			"x-hasura-allowed-roles": []string{"user"},
		},
	}, g.cfg.UpstreamJWTSecret)
}

// callUpstream performs one gateway→plugin request.
// body != nil is JSON-encoded. See the file header for credential rules.
func (g *gateway) callUpstream(ctx context.Context, r *http.Request, method, baseURL, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal upstream body: %w", err)
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rdr)
	if err != nil {
		return 0, nil, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	tid := saas.TenantFrom(r.Context())
	// nsk_ API keys pass through; session JWTs are replaced by a minted
	// service JWT (never forwarded — see the file header).
	forwardedKey := false
	if auth := r.Header.Get("Authorization"); auth != "" {
		cred := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if strings.HasPrefix(cred, saas.KeyPrefix) {
			req.Header.Set("Authorization", auth)
			forwardedKey = true
		}
	}
	if !forwardedKey && tid != "" && len(g.cfg.UpstreamJWTSecret) > 0 {
		if token, err := g.mintServiceJWT(tid); err == nil {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if tid != "" {
		req.Header.Set("X-Hasura-Tenant-Id", tid)
	}

	resp, err := upstreamClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("upstream %s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("read upstream response: %w", err)
	}
	return resp.StatusCode, respBody, nil
}

// upstreamOK reports whether status is a 2xx success.
func upstreamOK(status int) bool { return status >= 200 && status < 300 }

// bytesReader wraps a byte slice for request bodies.
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// readAllLimited reads a response body with the standard 4 MiB cap.
func readAllLimited(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 4<<20))
}
