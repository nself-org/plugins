package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	sdk "github.com/nself-org/plugin-sdk"
)

// trustStatus is the guard result reported by the status endpoint.
type trustStatus struct {
	OK       bool   `json:"ok"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Group    string `json:"group,omitempty"`
	Override bool   `json:"override_public_repos"`
	Warning  string `json:"warning,omitempty"`
	// Stopped is true when this check stopped a running runner.
	Stopped bool `json:"stopped_runner,omitempty"`
}

// respondTrustError writes a guard refusal as JSON with its stable code
// (HTTP 409 for runner_group_public). Other errors become a 500.
func respondTrustError(w http.ResponseWriter, err error) {
	var te *TrustError
	if errors.As(err, &te) {
		sdk.Respond(w, te.Status, map[string]string{"error": te.Message, "code": te.Code})
		return
	}
	sdk.Error(w, http.StatusInternalServerError, err)
}

// stopRunnerProcess stops the running runner and its children: SIGTERM, then
// SIGKILL for survivors after a grace period. It returns without error only
// once the process tree is verified gone, and reports whether a runner was
// running.
func (h *Handler) stopRunnerProcess() (pid int, running bool, err error) {
	h.state.mu.Lock()
	running = h.state.Running
	pid = h.state.PID
	h.state.mu.Unlock()
	if !running {
		return pid, false, nil
	}
	if err := stopRunnerTree(pid); err != nil {
		return pid, true, fmt.Errorf("stop runner process group %d: %w", pid, err)
	}
	return pid, true, nil
}

// enforceTrust re-runs the guard. A runner group or repo that has become open
// to public repositories stops a running runner unless the override is set.
// A check that cannot complete is reported but does not stop a running runner.
func (h *Handler) enforceTrust(ctx context.Context) trustStatus {
	cfg := TrustConfigFromEnv()
	res, err := CheckTrust(ctx, cfg)
	if err == nil {
		return trustStatus{OK: true, Group: res.Group, Override: res.Override, Warning: res.Warning}
	}
	ts := trustStatus{Message: err.Error(), Override: cfg.AllowPublic}
	var te *TrustError
	if errors.As(err, &te) {
		ts.Code, ts.Message = te.Code, te.Message
	}
	if stopsRunner(ts.Code) {
		pid, was, serr := h.stopRunnerProcess()
		switch {
		case serr != nil:
			log.Printf("github-runner: trust guard could not stop runner: %v", serr)
		case was:
			ts.Stopped = true
			log.Printf("github-runner: runner group or repo failed the trust check; stopped runner (PID %d)", pid)
		}
	}
	return ts
}

// trustCheckTimeout bounds the guard inside the status endpoint.
const trustCheckTimeout = 35 * time.Second
