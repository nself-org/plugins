// Package forgejo implements `nself ci forgejo`: a Forgejo server and runner
// health check for the ops profile.
//
// Purpose: behaviour-preserving port of cli cmd/commands/ci_forgejo.go
// (P7-CANON-09). Queries the Forgejo /-/health endpoint and the
// forgejo-runner container state (docker inspect) and prints a status table.
// Inputs:  Config (URL, runner container name); optional
// NSELF_FORGEJO_ADMIN_USER and NSELF_FORGEJO_ADMIN_PASSWORD for the API probe.
// Outputs: the status table on stdout, warnings on stderr; never an error
// (core returned nil for every outcome, so the exit code is always 0).
// Constraints: no Forgejo token; docker found by PATH lookup as in core.
// Temporary duplicate of the core command, deleted with it at P7-SHIP-09.
// SPORT: F08-SERVICE-INVENTORY (forgejo, ops profile, port 3844)
package forgejo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/clui"
)

// DefaultURL is the Forgejo base URL used when --url is not given.
const DefaultURL = "http://localhost:3844"

// Config holds the parsed flags.
type Config struct {
	URL    string // Forgejo base URL
	Runner string // runner container name; empty probes the common names
}

// healthResponse is the subset of /-/health we care about.
type healthResponse struct {
	Healthy bool `json:"healthy"`
}

// Run prints the Forgejo stack status.
func Run(cfg Config) error {
	baseURL := strings.TrimRight(cfg.URL, "/")
	runnerContainer := cfg.Runner

	client := &http.Client{Timeout: 10 * time.Second}

	clui.Section("Forgejo CI stack (ops profile)")

	serverOK, serverMsg := probeServer(client, baseURL)
	runnerContainer, runnerStatus := probeRunner(runnerContainer)
	jobsInfo := probeAPI(client, baseURL, serverOK)

	serverIcon := "✗"
	if serverOK {
		serverIcon = "✓"
	}
	runnerIcon := "✗"
	if runnerStatus == "running" {
		runnerIcon = "✓"
	}

	fmt.Printf("  %s Forgejo server   %s   %s\n", serverIcon, baseURL, serverMsg)
	fmt.Printf("  %s Forgejo runner   container=%s   state=%s\n", runnerIcon, runnerContainer, runnerStatus)
	fmt.Printf("    API auth         %s\n", jobsInfo)
	fmt.Println()

	if !serverOK {
		clui.Warn("Forgejo server unreachable. Is the ops profile running? Try: nself start --profile ops")
	}
	if runnerStatus != "running" {
		clui.Warn("Forgejo runner is not running. Check: docker logs " + runnerContainer)
	}
	if serverOK && runnerStatus == "running" {
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

// probeRunner returns the container name used and its state (docker inspect).
func probeRunner(runner string) (string, string) {
	state := "unknown"
	if runner == "" {
		// Try common project prefix patterns.
		for _, candidate := range []string{"nself_forgejo_runner", "app_forgejo_runner"} {
			out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", candidate).Output()
			if err == nil {
				runner = candidate
				state = strings.TrimSpace(string(out))
				break
			}
		}
		if runner == "" {
			state = "not found (docker inspect failed — is Docker running?)"
		}
	} else if out, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", runner).Output(); err == nil {
		state = strings.TrimSpace(string(out))
	}
	return runner, state
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
