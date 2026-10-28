// Package forgejo implements the Forgejo server health check for the ops profile.
// The plugin has no in-stack runner (P7-TRUST-21); external runner health is
// managed on its CI node. The legacy gate's exit-zero contract is retained.
package forgejo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/clui"
)

// DefaultURL is the Forgejo base URL used when --url is not given.
const DefaultURL = "http://localhost:3844"

// Config holds the parsed flags.
type Config struct {
	URL string // Forgejo base URL
}

// healthResponse is the subset of /-/health we care about.
type healthResponse struct {
	Healthy bool `json:"healthy"`
}

// Run prints the Forgejo stack status.
func Run(cfg Config) error {
	baseURL := strings.TrimRight(cfg.URL, "/")

	client := &http.Client{Timeout: 10 * time.Second}

	clui.Section("Forgejo CI stack (ops profile)")

	serverOK, serverMsg := probeServer(client, baseURL)
	jobsInfo := probeAPI(client, baseURL, serverOK)

	serverIcon := "✗"
	if serverOK {
		serverIcon = "✓"
	}
	fmt.Printf("  %s Forgejo server   %s   %s\n", serverIcon, baseURL, serverMsg)
	fmt.Printf("    API auth         %s\n", jobsInfo)
	fmt.Println()

	if !serverOK {
		clui.Warn("Forgejo server unreachable. Is the ops profile running? Try: nself start --profile ops")
	}
	if serverOK {
		clui.Success("Forgejo CI stack is healthy.")
	}
	return nil
}

// probeServer checks the liveness endpoint.
func probeServer(client *http.Client, baseURL string) (bool, string) {
	resp, err := client.Get(baseURL + "/-/health")
	if err != nil {
		return false, "unreachable"
	}
	defer func() { _ = resp.Body.Close() }()
	var h healthResponse
	if json.NewDecoder(resp.Body).Decode(&h) == nil && h.Healthy {
		return true, "healthy"
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// probeAPI reports the optional admin-authenticated API probe.
func probeAPI(client *http.Client, baseURL string, serverOK bool) string {
	info := "n/a (set NSELF_FORGEJO_ADMIN_USER + NSELF_FORGEJO_ADMIN_PASSWORD)"
	user := os.Getenv("NSELF_FORGEJO_ADMIN_USER")
	pass := os.Getenv("NSELF_FORGEJO_ADMIN_PASSWORD")
	if user == "" || pass == "" || !serverOK {
		return info
	}
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/repos/search?limit=0", nil)
	req.SetBasicAuth(user, pass)
	r, err := client.Do(req)
	if err != nil {
		return info
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode == http.StatusOK {
		return "API reachable (admin authenticated)"
	}
	return fmt.Sprintf("API returned HTTP %d", r.StatusCode)
}
