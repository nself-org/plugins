// Package hasura provides a minimal client for the Hasura Metadata API v3.
// Used by the controller to register and deregister per-project Postgres sources.
// Each project gets its own Hasura source — not a shared source with multiple schemas.
package hasura

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a thin wrapper around the Hasura Metadata API.
type Client struct {
	endpoint    string
	adminSecret string
	httpClient  *http.Client
}

// NewClient creates a Hasura Metadata API client.
func NewClient(endpoint, adminSecret string) *Client {
	return &Client{
		endpoint:    endpoint,
		adminSecret: adminSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AddSourceInput holds parameters for pg_add_source.
type AddSourceInput struct {
	Name        string
	DatabaseURL string
}

// AddSource registers a new Postgres source in Hasura metadata.
// Idempotent: if the source already exists, it is a no-op.
func (c *Client) AddSource(ctx context.Context, input AddSourceInput) error {
	payload := map[string]interface{}{
		"type": "pg_add_source",
		"args": map[string]interface{}{
			"name": input.Name,
			"configuration": map[string]interface{}{
				"connection_info": map[string]interface{}{
					"database_url": input.DatabaseURL,
					"pool_settings": map[string]interface{}{
						"max_connections":    10,
						"idle_timeout":       180,
						"connection_lifetime": 600,
					},
				},
			},
		},
	}

	if err := c.post(ctx, payload); err != nil {
		// Source already exists is not a fatal error.
		if isAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("pg_add_source %q: %w", input.Name, err)
	}
	return nil
}

// DropSource deregisters a Postgres source from Hasura metadata.
// Idempotent: if the source does not exist, it is a no-op.
func (c *Client) DropSource(ctx context.Context, sourceName string) error {
	payload := map[string]interface{}{
		"type": "pg_drop_source",
		"args": map[string]interface{}{
			"name":    sourceName,
			"cascade": true,
		},
	}

	if err := c.post(ctx, payload); err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("pg_drop_source %q: %w", sourceName, err)
	}
	return nil
}

// post sends a request to the Hasura Metadata API /v1/metadata endpoint.
func (c *Client) post(ctx context.Context, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/metadata", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hasura-Admin-Secret", c.adminSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hasura metadata API %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// isAlreadyExists returns true when the Hasura error indicates the source already exists.
func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "already exists") || contains(s, "source already added")
}

// isNotFound returns true when the Hasura error indicates the source does not exist.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "source not found") || contains(s, "not found")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
