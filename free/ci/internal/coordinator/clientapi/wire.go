package clientapi

// Purpose: the wire types of the coordinator user API (Epic D14) and the Backend that serves them, shared
// by the server handlers, the store backend and internal/coordclient.
// Inputs:  none.
// Outputs: SubmitRequest/SubmitResult, RunItem, LogQuery, Backend.
// Constraints: JSON field names are the contract; RunView and logs.Line keep their own tags; nothing here
//          carries a secret, a token or a log text other than stored (already redacted) log lines.

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Paths of the built-in routes (the client builds its URLs from the same constants).
const (
	PathRuns     = "/v1/runs"
	PathRunsPfx  = "/v1/runs/"
	PathLogsPfx  = "/v1/logs/"
	PathEvidence = "/v1/evidence/"
	PathPing     = "/v1/ping"
)

// SubmitRequest is POST /v1/runs: a revision of an owner/repo project, never a policy or a token.
type SubmitRequest struct {
	Project  string `json:"project"`
	Revision string `json:"revision"`
	Pipeline string `json:"pipeline,omitempty"`
}

// SubmitResult answers a submit: ID names the run for every other route.
type SubmitResult struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Outcome string `json:"outcome"` // enqueued, duplicate, coalesced or replay
}

// RunItem is one run as the user API shows it: the read model of `ci runs` plus the project that owns it.
type RunItem struct {
	cirun.RunView
	Project string `json:"project,omitempty"` // "" for a run no intake row owns (admin only)
}

// LogQuery selects the lines of one job's stored log.
type LogQuery struct {
	Run     string
	Job     string
	Attempt int   // 0 = the newest attempt
	After   int64 // first seq wanted is After+1
	Follow  bool
}

// Backend is what the handlers need from the coordinator; StoreBackend implements it over the store.
type Backend interface {
	// Submit stores a trigger for the project the caller was already authorized for.
	Submit(ctx context.Context, r SubmitRequest, by string) (SubmitResult, error)
	// List returns the newest runs, newest first.
	List(ctx context.Context, limit int) ([]RunItem, error)
	// Get resolves a run by id, unique prefix or "latest".
	Get(ctx context.Context, ref string) (RunItem, bool, error)
	// Cancel asks every live attempt of the run to stop and returns how many it asked.
	Cancel(ctx context.Context, ref, by string) (int, error)
	// Logs streams stored log lines; ok is false when the run or job has none.
	Logs(ctx context.Context, q LogQuery, fn func(logs.Line) error) (ok bool, err error)
	// Evidence returns the stored run evidence document exactly as it was signed.
	Evidence(ctx context.Context, ref string) (json.RawMessage, error)
}
