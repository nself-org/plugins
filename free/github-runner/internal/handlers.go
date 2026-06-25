package internal

import (

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
