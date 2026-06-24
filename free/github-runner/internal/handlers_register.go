package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	pat := os.Getenv("GITHUB_RUNNER_PAT")
	if pat == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("GITHUB_RUNNER_PAT is not set"))
		return
	}

	org := os.Getenv("GITHUB_RUNNER_ORG")
	if org == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("GITHUB_RUNNER_ORG is not set"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	token, err := fetchRegistrationToken(ctx, pat, org)
	if err != nil {
		sdk.Error(w, http.StatusBadGateway, fmt.Errorf("fetch registration token: %w", err))
		return
	}

	dir := runnerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create runner dir: %w", err))
		return
	}

	name := os.Getenv("GITHUB_RUNNER_NAME")
	if name == "" {
		hostname, _ := os.Hostname()
		name = "nself-" + hostname
	}

	labels := os.Getenv("GITHUB_RUNNER_LABELS")
	if labels == "" {
		labels = "self-hosted,linux,x64"
	}

	group := os.Getenv("GITHUB_RUNNER_GROUP")
	if group == "" {
		group = "Default"
	}

	configScript := filepath.Join(dir, "config.sh")
	if _, err := os.Stat(configScript); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("config.sh not found in %s — install the runner binary first", dir))
		return
	}

	args := []string{
		"--unattended",
		"--replace",
		"--url", fmt.Sprintf("https://github.com/%s", org),
		"--token", token,
		"--name", name,
		"--labels", labels,
		"--runnergroup", group,
	}

	cmd := exec.CommandContext(ctx, configScript, args...)
	cmd.Dir = dir
	// Pass a filtered env (no HASURA/DATABASE/NSELF_LICENSE/STRIPE/secrets) so
	// the runner config step cannot read the plugin's credentials.
	cmd.Env = runnerEnv()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("config.sh failed: %w — stderr: %s", err, stderr.String()))
		return
	}

	h.state.mu.Lock()
	h.state.Registered = true
	h.state.Name = name
	h.state.Labels = labels
	h.state.Org = org
	h.state.mu.Unlock()

	log.Printf("github-runner: registered runner %q for org %q", name, org)

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"registered": true,
		"name":       name,
		"labels":     labels,
		"org":        org,
	})
}

// fetchRegistrationToken calls the GitHub REST API to get a short-lived
// registration token for the given org.
func fetchRegistrationToken(ctx context.Context, pat, org string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/actions/runners/registration-token", org)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if payload.Token == "" {
		return "", fmt.Errorf("empty token in GitHub API response")
	}

	return payload.Token, nil
}

// --------------------------------------------------------------------------
// POST /v1/start
// --------------------------------------------------------------------------

// Start launches the runner subprocess (./run.sh) in the runner directory.
