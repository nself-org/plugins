// Package client is the Go caller for the storage plugin's /storage/* HTTP API.
//
// Purpose: send X-Internal-Token (PLUGIN_INTERNAL_SECRET) on every call, so a server-side caller is not refused
// with 401 by the plugin's internal-secret gate (D-0396, D-0648).
// Inputs: the plugin base URL, the internal secret, an optional source account id.
// Outputs: typed results, or *StatusError for any non-2xx answer.
// Constraints: server-side only. The secret lets a caller name any account, so never ship it to a browser or a
// mobile app.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	secretHeader = "X-Internal-Token"
	// legacySecretHeader is sent as well for one release, so this client still works against a storage plugin that
	// only reads the old name (D-0198).
	legacySecretHeader = "X-Plugin-Secret"
	accountHeader      = "X-Hasura-Source-Account-Id"
	maxErrBody         = 4 << 10
)

// ErrNoSecret is returned when no internal secret is available. Failing here is clearer than a 401 or 503 later.
var ErrNoSecret = errors.New("storage client: PLUGIN_INTERNAL_SECRET is empty")

// StatusError is a non-2xx answer from the plugin.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("storage: HTTP %d: %s", e.Code, e.Body) }

// Client calls one storage plugin. Safe for concurrent use.
type Client struct {
	base    string
	secret  string
	account string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithAccount sets X-Hasura-Source-Account-Id (the tenant) on every call. Unset means the plugin default.
func WithAccount(id string) Option { return func(c *Client) { c.account = id } }

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New returns a Client for the plugin at baseURL (for example http://storage:9007) using secret.
func New(baseURL, secret string, opts ...Option) (*Client, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, ErrNoSecret
	}
	c := &Client{base: strings.TrimRight(baseURL, "/"), secret: secret, http: &http.Client{Timeout: 5 * time.Minute}}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// NewFromEnv is New with the secret read from PLUGIN_INTERNAL_SECRET, the value `nself init` writes to .env.
func NewFromEnv(baseURL string, opts ...Option) (*Client, error) {
	return New(baseURL, os.Getenv("PLUGIN_INTERNAL_SECRET"), opts...)
}

// Do sends one request with the secret (and account) headers set. A non-2xx answer becomes *StatusError;
// on success the caller closes the response body.
func (c *Client) Do(ctx context.Context, method, path, contentType string, body io.Reader, length int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if length >= 0 && body != nil {
		req.ContentLength = length
	}
	req.Header.Set(secretHeader, c.secret)
	req.Header.Set(legacySecretHeader, c.secret)
	if c.account != "" {
		req.Header.Set(accountHeader, c.account)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
		return nil, &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	return resp, nil
}

func objectPath(bucket, key string) string {
	return "/storage/buckets/" + url.PathEscape(bucket) + "/objects/" + url.PathEscape(key)
}

// CreateBucket creates a bucket and returns its id. region may be empty.
func (c *Client) CreateBucket(ctx context.Context, name, region string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"name": name, "region": region})
	resp, err := c.Do(ctx, http.MethodPost, "/storage/buckets", "application/json", strings.NewReader(string(payload)), int64(len(payload)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("storage: decode create-bucket answer: %w", err)
	}
	return out.ID, nil
}

// PutObject uploads body as bucket/key. Pass the byte length, or -1 when unknown (sent chunked).
func (c *Client) PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader, length int64) error {
	resp, err := c.Do(ctx, http.MethodPut, objectPath(bucket, key), contentType, body, length)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// GetObject streams bucket/key. The caller closes the returned reader.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	resp, err := c.Do(ctx, http.MethodGet, objectPath(bucket, key), "", nil, -1)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// DeleteObject removes bucket/key.
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	resp, err := c.Do(ctx, http.MethodDelete, objectPath(bucket, key), "", nil, -1)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
