package internal

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	sdk "github.com/nself-org/plugin-sdk"
)

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
