// Package hetzner provides a minimal Hetzner Cloud API client for nself-cloud
// server provisioning and deprovisioning.
package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

const (
	baseURL    = "https://api.hetzner.cloud/v1"
	maxRetries = 3
)

// ServerOpts describes the server to create.
type ServerOpts struct {
	Name       string            // unique name (slug-based)
	ServerType string            // e.g. "cx23"
	Image      string            // e.g. "ubuntu-22.04"
	Location   string            // e.g. "fsn1"
	SSHKeyID   int64             // 0 = no SSH key
	UserData   string            // cloud-init script
	Labels     map[string]string // metadata labels for idempotency
}

// ServerResult is returned on successful creation.
type ServerResult struct {
	ServerID int64
	IPv4     string
	IPv6     string
}

// Client is a minimal Hetzner Cloud API client.
// Token is read from HETZNER_NSELF_TOKEN env var.
type Client struct {
	token      string
	httpClient *http.Client
}

// New creates a Client using HETZNER_NSELF_TOKEN from env.
func New() (*Client, error) {
	token := os.Getenv("HETZNER_NSELF_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("hetzner: HETZNER_NSELF_TOKEN not set")
	}
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// NewWithToken creates a Client with an explicit token (useful for tests).
func NewWithToken(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateServer provisions a new Hetzner server and returns its ID and IPs.
// Retried up to maxRetries times with exponential backoff.
// If a server with the same name already exists (idempotency), returns its details.
func (c *Client) CreateServer(ctx context.Context, opts ServerOpts) (ServerResult, error) {
	body := map[string]interface{}{
		"name":        opts.Name,
		"server_type": opts.ServerType,
		"image":       opts.Image,
		"location":    opts.Location,
		"user_data":   opts.UserData,
		"labels":      opts.Labels,
	}
	if opts.SSHKeyID > 0 {
		body["ssh_keys"] = []interface{}{opts.SSHKeyID}
	}

	var result ServerResult
	err := c.retry(ctx, maxRetries, func() error {
		resp, err := c.doJSON(ctx, http.MethodPost, "/servers", body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("hetzner: read create response: %w", err)
		}

		// 409 Conflict = server name already exists — idempotent: fetch existing.
		if resp.StatusCode == http.StatusConflict {
			existing, err := c.getServerByName(ctx, opts.Name)
			if err != nil {
				return fmt.Errorf("hetzner: idempotency fetch: %w", err)
			}
			result = existing
			return nil
		}

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("hetzner: create server HTTP %d: %s", resp.StatusCode, string(raw))
		}

		var parsed struct {
			Server struct {
				ID              int64 `json:"id"`
				PublicNet       struct {
					IPv4 struct {
						IP string `json:"ip"`
					} `json:"ipv4"`
					IPv6 struct {
						IP string `json:"ip"`
					} `json:"ipv6"`
				} `json:"public_net"`
			} `json:"server"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("hetzner: parse create response: %w", err)
		}
		result = ServerResult{
			ServerID: parsed.Server.ID,
			IPv4:     parsed.Server.PublicNet.IPv4.IP,
			IPv6:     parsed.Server.PublicNet.IPv6.IP,
		}
		return nil
	})
	return result, err
}

// DeleteServer deletes a Hetzner server by ID (compensating action on saga failure).
// Returns nil if the server is already gone (idempotent).
func (c *Client) DeleteServer(ctx context.Context, serverID int64) error {
	return c.retry(ctx, maxRetries, func() error {
		resp, err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/servers/%d", serverID), nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil // already gone — idempotent
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			raw, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("hetzner: delete server %d HTTP %d: %s", serverID, resp.StatusCode, string(raw))
		}
		return nil
	})
}

// ServerStatus represents the running state of a Hetzner server.
type ServerStatus string

const (
	ServerStatusRunning     ServerStatus = "running"
	ServerStatusInitializing ServerStatus = "initializing"
	ServerStatusStarting    ServerStatus = "starting"
	ServerStatusStopping    ServerStatus = "stopping"
	ServerStatusOff         ServerStatus = "off"
	ServerStatusDeleting    ServerStatus = "deleting"
	ServerStatusUnknown     ServerStatus = "unknown"
)

// GetServerStatus fetches the current status of a server by ID.
func (c *Client) GetServerStatus(ctx context.Context, serverID int64) (ServerStatus, error) {
	var status ServerStatus
	err := c.retry(ctx, maxRetries, func() error {
		resp, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/servers/%d", serverID), nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("hetzner: read status response: %w", err)
		}
		if resp.StatusCode == http.StatusNotFound {
			status = ServerStatusUnknown
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("hetzner: get server %d HTTP %d: %s", serverID, resp.StatusCode, string(raw))
		}

		var parsed struct {
			Server struct {
				Status string `json:"status"`
			} `json:"server"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("hetzner: parse status response: %w", err)
		}
		status = ServerStatus(parsed.Server.Status)
		return nil
	})
	return status, err
}

// getServerByName fetches an existing server by name for idempotency.
func (c *Client) getServerByName(ctx context.Context, name string) (ServerResult, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/servers?name=%s", name), nil)
	if err != nil {
		return ServerResult{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ServerResult{}, fmt.Errorf("hetzner: read list response: %w", err)
	}

	var parsed struct {
		Servers []struct {
			ID        int64 `json:"id"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
				IPv6 struct {
					IP string `json:"ip"`
				} `json:"ipv6"`
			} `json:"public_net"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ServerResult{}, fmt.Errorf("hetzner: parse list response: %w", err)
	}
	if len(parsed.Servers) == 0 {
		return ServerResult{}, fmt.Errorf("hetzner: server %q not found after conflict", name)
	}
	s := parsed.Servers[0]
	return ServerResult{
		ServerID: s.ID,
		IPv4:     s.PublicNet.IPv4.IP,
		IPv6:     s.PublicNet.IPv6.IP,
	}, nil
}

// doJSON executes an HTTP request with JSON body and Bearer auth.
func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("hetzner: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("hetzner: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// retry executes fn up to n times with exponential backoff.
// Retryable: 429 Too Many Requests and 5xx errors.
// Non-retryable: context cancellation, 4xx (except 409 handled upstream).
func (c *Client) retry(ctx context.Context, n int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < n; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if attempt < n-1 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return fmt.Errorf("hetzner: after %d attempts: %w", n, lastErr)
}
