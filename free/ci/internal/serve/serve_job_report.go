package serve

// Purpose: post-job reporting for a CI gate run — GitHub commit status, completion-event emission, and small string/exec/binary-resolution helpers used by serve_job.go.
// Inputs: job/status details (owner, repo, sha, state, description) and a completionEvent.
// Outputs: a posted GitHub commit status and/or an HTTP POST to NSELF_CI_EVENT_SINK.
// Constraints: split out of serve_job.go as a pure move (CLI-R12); no behavior change.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/clui"
)

// postCommitStatus posts a nself-ci GitHub commit status via `gh api` (OAuth).
// Mirrors the logic in plugins/free/ci/internal/legacy/status.go without importing it
// (different Go module — invoked as a binary, not a library).
func postCommitStatus(owner, repo, sha, state, description string) error {
	if owner == "" || repo == "" || sha == "" {
		return fmt.Errorf("owner/repo/sha required")
	}
	endpoint := fmt.Sprintf("repos/%s/%s/statuses/%s", owner, repo, sha)
	args := []string{
		"api", "--method", "POST", endpoint,
		"-f", "state=" + state,
		"-f", "context=nself-ci",
		"-f", "description=" + truncateStr(description, 140),
	}
	var stderr bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api: %w — %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// completionEvent is posted to NSELF_CI_EVENT_SINK when a job finishes.
// This is the seam for nself-cron-monitor (port 3839).
type completionEvent struct {
	Repo     string `json:"repo"`
	Ref      string `json:"ref"`
	SHA      string `json:"sha"`
	Status   string `json:"status"`   // "success" | "failure" | "error"
	Duration string `json:"duration"` // e.g. "1m23s"
	Summary  string `json:"summary,omitempty"`
}

// emitEvent POSTs the completion event to NSELF_CI_EVENT_SINK if configured.
// Best-effort: errors are logged but never propagate.
func emitEvent(ev completionEvent) {
	sink := os.Getenv("NSELF_CI_EVENT_SINK")
	if sink == "" {
		return
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sink, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := eventClient.Do(req)
	if err != nil {
		clui.Warn(fmt.Sprintf("[ci-serve] event sink %s: %v", sink, err))
		return
	}
	_ = resp.Body.Close()
}

// runCmd executes a command and returns combined stdout+stderr.
func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// splitFullName splits "owner/repo" into (owner, repo).
func splitFullName(full string) (string, string) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return full, full
}

// extractSummary pulls the last non-empty line of nself-ci output as the summary.
func extractSummary(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return truncateStr(l, 140)
		}
	}
	return "gate complete"
}

// truncateStr truncates s to max runes, appending "…" if truncated.
func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// eventClient posts completion events. Same 30s default as the core client
// (NSELF_HTTP_TIMEOUT_DEFAULT, integer seconds) so behaviour matches core.
var eventClient = &http.Client{Timeout: envSeconds("NSELF_HTTP_TIMEOUT_DEFAULT", 30*time.Second)}

// envSeconds reads an integer-second env var; unset, non-numeric or
// non-positive values fall back.
func envSeconds(key string, fallback time.Duration) time.Duration {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}

// gateBinary returns the running nself-ci binary. The job runs the gate by
// exec of this executable, never by PATH lookup or a go build, so a hijacked
// PATH or a source checkout cannot change what executes (P7-CANON-09).
func gateBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}
