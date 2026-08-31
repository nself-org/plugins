// Package ghost implements a Ghost publishing client for the Admin API.
//
// Ghost uses a JWT-authenticated Admin API. The Admin API key is provisioned
// in Ghost Admin → Integrations as a string in the form "<keyId>:<secret>"
// where secret is hex-encoded. Each request signs a short-lived JWT with HS256
// using the decoded secret bytes; the JWT header includes "kid" = keyId.
//
// This package intentionally signs the JWT with only crypto/hmac + crypto/sha256
// so the plugin avoids a dedicated JWT dependency for what is a 20-line signer.
package ghost

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared/httpclient"
)

// ghostFallbackClient is the default HTTP client used when Client.HTTPClient
// is nil. 30s budget per spec.
var ghostFallbackClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

// Client publishes posts to a Ghost CMS instance via the Admin API.
type Client struct {
	// BaseURL is the Ghost site root, e.g. https://example.ghost.io or
	// https://blog.example.com. The client appends /ghost/api/admin paths.
	BaseURL string

	// AdminAPIKey is in the form "keyId:hexSecret".
	AdminAPIKey string

	// HTTPClient is the transport used for API calls. If nil, a default client
	// with a 30 second timeout is used.
	HTTPClient *http.Client

	// now is injected for deterministic JWT timestamps in tests.
	now func() time.Time
}

// NewClient returns a Ghost client with sensible defaults.
func NewClient(baseURL, adminAPIKey string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		AdminAPIKey: adminAPIKey,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
		now:         time.Now,
	}
}

// NewPostArgs is the body of an Admin POST /posts/ call.
type NewPostArgs struct {
	Title  string
	HTML   string   // Rendered HTML body. Ghost will convert to mobiledoc internally.
	Tags   []string // Tag names. Ghost creates missing tags automatically.
	Status string   // "published" (default), "draft", "scheduled"
}

// NewPostResult is the outcome of a successful post creation.
type NewPostResult struct {
	PostID string // Ghost post ID (ObjectId-style string)
	URL    string // Resolved post URL
}

// NewPost publishes a new post via POST /ghost/api/admin/posts/?source=html.
// The source=html query parameter tells Ghost to convert the provided HTML
// into its internal mobiledoc format.
func (c *Client) NewPost(ctx context.Context, args NewPostArgs) (*NewPostResult, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("ghost: base URL is empty")
	}
	if c.AdminAPIKey == "" {
		return nil, fmt.Errorf("ghost: admin API key is required")
	}

	status := args.Status
	if status == "" {
		status = "published"
	}

	tagObjs := make([]map[string]string, 0, len(args.Tags))
	for _, t := range args.Tags {
		tagObjs = append(tagObjs, map[string]string{"name": t})
	}

	payload := map[string]interface{}{
		"posts": []map[string]interface{}{
			{
				"title":  args.Title,
				"html":   args.HTML,
				"status": status,
				"tags":   tagObjs,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	token, err := c.signJWT()
	if err != nil {
		return nil, fmt.Errorf("sign JWT: %w", err)
	}

	url := c.BaseURL + "/ghost/api/admin/posts/?source=html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Version", "v5.0")
	req.Header.Set("Authorization", "Ghost "+token)
	req.Header.Set("User-Agent", "nself-post/1.0 (+https://nself.org)")

	client := c.HTTPClient
	if client == nil {
		client = ghostFallbackClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ghost POST /posts/: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp.StatusCode, raw)
	}

	var decoded struct {
		Posts []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(decoded.Posts) == 0 {
		return nil, fmt.Errorf("ghost returned no posts in response body")
	}
	return &NewPostResult{
		PostID: decoded.Posts[0].ID,
		URL:    decoded.Posts[0].URL,
	}, nil
}

// =============================================================================
// JWT signing
// =============================================================================

// signJWT builds and signs a Ghost Admin API JWT using the configured key.
// Ghost requires HS256 with:
//   - header.alg = "HS256"
//   - header.typ = "JWT"
//   - header.kid = keyId (first half of the admin API key)
//   - payload.iat = now (unix seconds)
//   - payload.exp = now + 5 minutes
//   - payload.aud = "/admin/" (Ghost v4+) — matches the API path root
//
// The secret is hex-decoded before signing.
func (c *Client) signJWT() (string, error) {
	keyID, secret, err := splitAdminKey(c.AdminAPIKey)
	if err != nil {
		return "", err
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	// Ghost validates iat strictly (must be <= server clock). Back-date iat
	// by 30 seconds so a small local clock skew (NTP drift, VM clock)
	// doesn't cause every post to be rejected as "future-dated". The exp
	// is 5 minutes out and unaffected by the skew.
	// SIEGE HIGH-1 remediation.
	const skewTolerance = 30 * time.Second
	iat := now().Add(-skewTolerance).Unix()
	exp := now().Add(5 * time.Minute).Unix()

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
		"kid": keyID,
	}
	payload := map[string]interface{}{
		"iat": iat,
		"exp": exp,
		"aud": "/admin/",
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	he := base64URLEncode(hb)
	pe := base64URLEncode(pb)
	signingInput := he + "." + pe

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	sig := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

// splitAdminKey parses "keyId:hexSecret" into (keyId, secretBytes).
func splitAdminKey(k string) (string, []byte, error) {
	parts := strings.SplitN(k, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, fmt.Errorf("admin API key must be in the form '<keyId>:<hexSecret>'")
	}
	secret, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("admin API key secret must be hex-encoded: %w", err)
	}
	return parts[0], secret, nil
}

// base64URLEncode is JWT base64url without padding.
func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// =============================================================================
// Error parsing
// =============================================================================

// parseAPIError converts a Ghost error JSON body into a useful Go error.
// Ghost errors look like: {"errors":[{"message":"...","type":"..."}]}.
func parseAPIError(status int, body []byte) error {
	var decoded struct {
		Errors []struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Context string `json:"context,omitempty"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &decoded); err == nil && len(decoded.Errors) > 0 {
		e := decoded.Errors[0]
		if e.Context != "" {
			return fmt.Errorf("ghost HTTP %d: %s (%s) — %s", status, e.Message, e.Type, e.Context)
		}
		return fmt.Errorf("ghost HTTP %d: %s (%s)", status, e.Message, e.Type)
	}
	return fmt.Errorf("ghost HTTP %d: %s", status, strings.TrimSpace(string(body)))
}
