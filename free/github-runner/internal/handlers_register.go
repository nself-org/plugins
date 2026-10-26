package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/nself-org/plugin-sdk"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Size-cap exception: single-responsibility HTTP route handler — 90L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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

	// Trusted-only guard: refuse a runner group (or repo) open to public
	// repositories before any registration token is requested.
	cfg := TrustConfigFromEnv()
	cfg.Registering = true // --replace moves the runner into the configured group
	trust, err := CheckTrust(ctx, cfg)
	if err != nil {
		respondTrustError(w, err)
		return
	}

	token, err := fetchRegistrationToken(ctx, cfg)
	if err != nil {
		sdk.Error(w, http.StatusBadGateway, fmt.Errorf("fetch registration token: %w", err))
		return
	}

	dir := runnerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create runner dir: %w", err))
		return
	}

	name := cfg.RunnerName

	labels := os.Getenv("GITHUB_RUNNER_LABELS")
	if labels == "" {
		labels = "self-hosted,linux,x64"
	}

	group := trust.Group

	configScript := filepath.Join(dir, "config.sh")
	if _, err := os.Stat(configScript); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("config.sh not found in %s — install the runner binary first", dir))
		return
	}

	args := []string{
		"--unattended",
		"--replace",
		"--url", registrationURL(cfg),
		"--token", token,
		"--name", name,
		"--labels", labels,
	}
	if cfg.Scope != "repo" {
		args = append(args, "--runnergroup", group)
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
		"warning":    trust.Warning,
	})
}

// registrationURL is the GitHub URL the runner registers against: the org, or
// the single repository for repo scope.
func registrationURL(cfg TrustConfig) string {
	if cfg.Scope == "repo" {
		return fmt.Sprintf("https://github.com/%s/%s", cfg.Org, cfg.Repo)
	}
	return fmt.Sprintf("https://github.com/%s", cfg.Org)
}

// fetchRegistrationToken calls the GitHub REST API to get a short-lived
// registration token for the org, or for the repository in repo scope.
func fetchRegistrationToken(ctx context.Context, cfg TrustConfig) (string, error) {
	pat := cfg.PAT
	url := fmt.Sprintf("%s/orgs/%s/actions/runners/registration-token", githubAPIBase, cfg.Org)
	if cfg.Scope == "repo" {
		url = fmt.Sprintf("%s/repos/%s/%s/actions/runners/registration-token", githubAPIBase, cfg.Org, cfg.Repo)
	}

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
