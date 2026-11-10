package server

// Purpose: the Client interface every `nself server` operation programs
// against, plus hetznerClient, its concrete implementation over the Hetzner
// Cloud API. The interface exists so cmd/commands/server_*.go can inject a
// fake in tests (mirroring internal/access.Transport) and so orchestration
// logic in provision.go/resize.go/destroy.go never depends on HTTP directly.
// Inputs: a Hetzner Cloud API token (see token.go for env-var resolution).
// Outputs: typed Server/ServerType/Image/Action/PrimaryIP values, or a
// wrapped error describing what the API call was and why it failed.
// Constraints: hetznerAPIBaseURL and hetznerHTTPClient are var indirections
// — same pattern as internal/access/hetzner_mismatch.go — so tests point at
// an httptest.Server instead of the real Hetzner Cloud API. No test in this
// package or its callers may reach the real API.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	hetznerAPIBaseURL = "https://api.hetzner.cloud/v1"
	hetznerHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// Client is every Hetzner Cloud operation `nself server` needs. Introducing
// a second provider later means adding a second implementation of this
// interface, not touching provision.go/resize.go/destroy.go/list.go.
type Client interface {
	GetPricing(ctx context.Context) (*Pricing, error)
	ListLocations(ctx context.Context) ([]Location, error)
	ListDatacenters(ctx context.Context) ([]Datacenter, error)
	CreateSSHKey(ctx context.Context, name, publicKey string, labels map[string]string) (*SSHKey, error)
	DeleteSSHKey(ctx context.Context, id int64) error
	ListSSHKeysByLabel(ctx context.Context, selector string) ([]SSHKey, error)
	ProbeWriteScope(ctx context.Context) (string, error)
	CreateServer(ctx context.Context, req ProvisionRequest) (*Server, error)
	ListServers(ctx context.Context, opts ListOptions) ([]Server, error)
	GetServer(ctx context.Context, id int64) (*Server, error)
	DeleteServer(ctx context.Context, id int64) error
	ListServerTypes(ctx context.Context) ([]ServerType, error)
	ChangeServerType(ctx context.Context, serverID int64, targetType string, upgradeDisk bool) (*Action, error)
	CreateSnapshot(ctx context.Context, serverID int64, description string) (*Image, *Action, error)
	GetImage(ctx context.Context, id int64) (*Image, error)
	GetAction(ctx context.Context, id int64) (*Action, error)
	ListPrimaryIPs(ctx context.Context, serverID int64) ([]PrimaryIP, error)
	SetPrimaryIPAutoDelete(ctx context.Context, ipID int64, autoDelete bool) error
}

// hetznerClient is the real Client, talking to api.hetzner.cloud/v1.
type hetznerClient struct {
	token string
}

// NewHetznerClient builds a Client backed by the live Hetzner Cloud API.
// token must be non-empty; resolve it with ResolveToken before calling this.
func NewHetznerClient(token string) Client {
	return &hetznerClient{token: token}
}

// apiError is Hetzner's standard error envelope, e.g.
// {"error":{"code":"invalid_input","message":"..."}}.
type apiError struct {
	Status int `json:"-"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// hetznerResponseError preserves the API code for the scope probe.
type hetznerResponseError struct {
	envelope apiError
	path     string // "METHOD /path" of the failed call; never carries the token
}

// Error describes the provider failure without exposing authentication material.
func (e *hetznerResponseError) Error() string {
	if e.path != "" {
		return fmt.Sprintf("hetzner API %s: %d %s: %s", e.path, e.envelope.Status, e.envelope.Error.Code, e.envelope.Error.Message)
	}
	return fmt.Sprintf("%d %s: %s", e.envelope.Status, e.envelope.Error.Code, e.envelope.Error.Message)
}

func apiBaseURL() (string, error) {
	base := os.Getenv("NSELF_INFRA_API_BASE")
	if base == "" {
		return hetznerAPIBaseURL, nil
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("NSELF_INFRA_API_BASE must be a loopback HTTP URL")
	}
	switch u.Hostname() {
	case "127.0.0.1", "::1", "localhost":
		return strings.TrimRight(base, "/"), nil
	default:
		return "", fmt.Errorf("NSELF_INFRA_API_BASE must use a loopback host")
	}
}

// do performs one Hetzner API call, decoding a JSON body into out (which may
// be nil for calls like DeleteServer that discard the response body).
func (c *hetznerClient) do(ctx context.Context, method, path string, body, out interface{}) error {
	_, err := c.doStatus(ctx, method, path, body, out)
	return err
}

func (c *hetznerClient) doStatus(ctx context.Context, method, path string, body, out interface{}) (status int, result error) {
	defer func() {
		if result != nil && c.token != "" {
			// Keep typed API errors available to the probe, with sanitized fields.
			if e, ok := result.(*hetznerResponseError); ok {
				e.envelope.Error.Code = strings.ReplaceAll(e.envelope.Error.Code, c.token, "[redacted]")
				e.envelope.Error.Message = strings.ReplaceAll(e.envelope.Error.Message, c.token, "[redacted]")
			} else {
				if strings.Contains(result.Error(), c.token) {
					result = fmt.Errorf("%s", strings.ReplaceAll(result.Error(), c.token, "[redacted]"))
				}
			}
		}
	}()
	base, err := apiBaseURL()
	if err != nil {
		return 0, err
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := *hetznerHTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("hetzner API %s %s: network error: %w", method, redactPath(path), err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("hetzner API %s %s: read response: %w", method, redactPath(path), err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Code != "" {
			ae.Status = resp.StatusCode
			return resp.StatusCode, &hetznerResponseError{envelope: ae, path: method + " " + redactPath(path)}
		}
		return resp.StatusCode, fmt.Errorf("hetzner API %s %s: HTTP %d", method, redactPath(path), resp.StatusCode)
	}

	if out == nil || len(raw) == 0 {
		return resp.StatusCode, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp.StatusCode, fmt.Errorf("hetzner API %s %s: decode response: %w", method, redactPath(path), err)
	}
	return resp.StatusCode, nil
}

// redactPath drops the query string so label selectors never reach error text.
func redactPath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}
