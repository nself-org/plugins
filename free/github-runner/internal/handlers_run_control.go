package internal

import (
	"errors"
	"fmt"
	sdk "github.com/nself-org/plugin-sdk"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// Size-cap exception: single-responsibility HTTP route handler — 72L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	// Trusted-only guard first: re-check the runner group (or repo) on every
	// start, including when a runner is already up, so a group that turned
	// public stops it.
	trust, err := CheckTrust(r.Context(), TrustConfigFromEnv())
	if err != nil {
		var te *TrustError
		if errors.As(err, &te) && stopsRunner(te.Code) {
			if pid, was, serr := h.stopRunnerProcess(); serr != nil {
				log.Printf("github-runner: trust guard could not stop runner: %v", serr)
			} else if was {
				log.Printf("github-runner: runner group or repo failed the trust check; stopped runner (PID %d)", pid)
			}
		}
		respondTrustError(w, err)
		return
	}

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
	// Pass a filtered env (no HASURA/DATABASE/NSELF_LICENSE/STRIPE/secrets) so
	// untrusted workflow code on the runner cannot exfiltrate the plugin's
	// credentials from the process environment.
	cmd.Env = runnerEnv()
	ownProcessGroup(cmd)

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
		"warning": trust.Warning,
	})
}

// --------------------------------------------------------------------------
// POST /v1/stop
// --------------------------------------------------------------------------

// Stop terminates the runner subprocess tree (SIGTERM, then SIGKILL) and
// responds only once it is gone.
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	pid, running, err := h.stopRunnerProcess()
	if !running {
		sdk.Error(w, http.StatusConflict, fmt.Errorf("runner is not running"))
		return
	}
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, err)
		return
	}

	log.Printf("github-runner: stopped runner (PID %d)", pid)

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
