package internal

import (
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

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
