package internal

import (
	"bufio"
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

const (
	defaultRunnerDir = ".nself/runners/github-runner"
	logTailLines     = 100
)

// Handler holds dependencies for the GitHub runner HTTP handlers.
type Handler struct {
	state *RunnerState
}

// NewHandler creates a Handler backed by the given RunnerState.
func NewHandler(state *RunnerState) *Handler {
	return &Handler{state: state}
}

// --------------------------------------------------------------------------
// GET /health
// --------------------------------------------------------------------------

// Health returns service liveness plus a brief runner status summary.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	snap := h.state.Snapshot()
	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "nself-github-runner",
		"runner": map[string]bool{
			"registered": snap.Registered,
			"running":    snap.Running,
		},
	})
}

// --------------------------------------------------------------------------
// GET /v1/status
// --------------------------------------------------------------------------

// statusResponse is the full runner status payload.
type statusResponse struct {
	Registered bool   `json:"registered"`
	Running    bool   `json:"running"`
	Name       string `json:"name"`
	Labels     string `json:"labels"`
	Org        string `json:"org"`
	PID        int    `json:"pid,omitempty"`
	RunnerDir  string `json:"runner_dir"`
}

// Status returns the current runner registration and process state.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	snap := h.state.Snapshot()
	sdk.Respond(w, http.StatusOK, statusResponse{
		Registered: snap.Registered,
		Running:    snap.Running,
		Name:       snap.Name,
		Labels:     snap.Labels,
		Org:        snap.Org,
		PID:        snap.PID,
		RunnerDir:  runnerDir(),
	})
}

// --------------------------------------------------------------------------
// POST /v1/register
// --------------------------------------------------------------------------

// Register fetches a registration token from the GitHub REST API and runs
// the runner config script to register the runner with the given org.
//
// Required env vars: GITHUB_RUNNER_PAT, GITHUB_RUNNER_ORG
// Optional env vars: GITHUB_RUNNER_NAME, GITHUB_RUNNER_LABELS, GITHUB_RUNNER_GROUP
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
	cmd.Env = os.Environ()

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
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	h.state.mu.Lock()
	if h.state.Running {
		h.state.mu.Unlock()
		sdk.Error(w, http.StatusConflict, fmt.Errorf("runner is already running (PID %d)", h.state.PID))
		return
	}
	h.state.mu.Unlock()

	dir := runnerDir()
	runScript := filepath.Join(dir, "run.sh")
	if _, err := os.Stat(runScript); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("run.sh not found in %s", dir))
		return
	}

	cmd := exec.Command(runScript)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	// Redirect stdout/stderr to the log file.
	logPath := logFilePath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create log dir: %w", err))
		return
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("open log file: %w", err))
		return
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("start runner: %w", err))
		return
	}

	pid := cmd.Process.Pid

	h.state.mu.Lock()
	h.state.Running = true
	h.state.PID = pid
	h.state.mu.Unlock()

	// Monitor the subprocess and clear state when it exits.
	go func() {
		defer logFile.Close()
		_ = cmd.Wait()

		h.state.mu.Lock()
		h.state.Running = false
		h.state.PID = 0
		h.state.mu.Unlock()

		log.Printf("github-runner: runner process exited (was PID %d)", pid)
	}()

	log.Printf("github-runner: runner started (PID %d)", pid)

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"started": true,
		"pid":     pid,
	})
}

// --------------------------------------------------------------------------
// POST /v1/stop
// --------------------------------------------------------------------------

// Stop sends SIGTERM to the runner subprocess.
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	h.state.mu.Lock()
	running := h.state.Running
	pid := h.state.PID
	h.state.mu.Unlock()

	if !running {
		sdk.Error(w, http.StatusConflict, fmt.Errorf("runner is not running"))
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("find process %d: %w", pid, err))
		return
	}

	if err := proc.Signal(os.Interrupt); err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("signal process %d: %w", pid, err))
		return
	}

	log.Printf("github-runner: sent interrupt to runner (PID %d)", pid)

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"stopped": true,
		"pid":     pid,
	})
}

// --------------------------------------------------------------------------
// GET /v1/logs
// --------------------------------------------------------------------------

// Logs returns the last 100 lines of the runner log file.
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	logPath := logFilePath()

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			sdk.Respond(w, http.StatusOK, map[string]interface{}{
				"lines": []string{},
				"path":  logPath,
			})
			return
		}
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("open log file: %w", err))
		return
	}
	defer f.Close()

	lines, err := tailLines(f, logTailLines)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("read log file: %w", err))
		return
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"lines": lines,
		"path":  logPath,
	})
}

// tailLines returns the last n lines from r.
func tailLines(r io.ReadSeeker, n int) ([]string, error) {
	// Read all content and return the last n lines. Log files for a
	// self-hosted runner are small enough that a full read is fine.
	scanner := bufio.NewScanner(r)
	var all []string
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// --------------------------------------------------------------------------
// Path helpers
// --------------------------------------------------------------------------

// runnerDir returns the directory where the runner binary and config live.
func runnerDir() string {
	if d := os.Getenv("GITHUB_RUNNER_DIR"); d != "" {
		return d
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, defaultRunnerDir)
}

// logFilePath returns the path to the runner log file.
func logFilePath() string {
	if p := os.Getenv("GITHUB_RUNNER_LOG_PATH"); p != "" {
		return p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	return filepath.Join(home, ".nself", "logs", "plugins", "github-runner", "runner.log")
}
