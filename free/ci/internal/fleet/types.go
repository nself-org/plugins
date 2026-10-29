package fleet

// Purpose: the document of `ci nodes status --json`: one Row per registered node, each carrying the
// node's contract:ci.demand v1 document when it publishes one.
// Constraints: additive changes only; slices are always arrays in JSON, never null.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
)

// Limits of one status pass (P7-AUTO-19).
const (
	// NodeCap is the longest the pass waits for one node.
	NodeCap = 8 * time.Second
	// Parallel is how many nodes are asked at once.
	Parallel = 8
	// MaxResults is how many recent results a row carries.
	MaxResults = 5
)

// Source values of a row.
const (
	SourceLive        = "live"
	SourceUnreachable = "unreachable"
)

// Watcher values of a row.
const (
	WatcherAlive   = "alive"
	WatcherStale   = "stale"
	WatcherNone    = "none"
	WatcherUnknown = "unknown"
)

// Result is one recent run from the local store, as `ci runs list` shows it.
type Result struct {
	ID       string `json:"id"`
	Project  string `json:"project"`
	Revision string `json:"revision"`
	Status   string `json:"status"`
}

// Row is the status of one node.
type Row struct {
	Node         string   `json:"node"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	AgentVersion string   `json:"agent_version"`
	Pools        []string `json:"pools"`
	// Source is live when the node answered, unreachable when it did not.
	Source string `json:"source"`
	// Unreachable is the reason the node did not answer ("" when it did).
	Unreachable string `json:"unreachable,omitempty"`
	// Ready is nil when readiness is unknown (unreachable or not published).
	Ready    *bool    `json:"ready"`
	ReadyWhy []string `json:"ready_why"`
	// Watcher is alive, stale, none (the host runs no watcher) or unknown (unreachable).
	Watcher  string `json:"watcher"`
	LastPass string `json:"last_pass,omitempty"`
	Queued   *int   `json:"queued"`
	Running  *int   `json:"running"`
	// Demand is the node's contract:ci.demand v1 document; nil when it publishes none.
	Demand      *demand.Document `json:"demand"`
	LastResults []Result         `json:"last_results"`
}

// Report is the data of the `ci nodes status --json` envelope.
type Report struct {
	Nodes       []Row `json:"nodes"`
	Unreachable int   `json:"unreachable"`
}
