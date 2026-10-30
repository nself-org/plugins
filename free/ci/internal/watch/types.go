// Package watch is the PERSONAL-mode trigger source `nself ci serve watch` (ADR 0032 decision 4,
// P7-AUTO-11): an outbound-only poller that discovers heads through the HOST-17 head scan, claims a
// commit on the provider's status ledger, runs it through the same coordinator path as
// `nself ci --trigger watch`, and posts the one result. It opens no listener and owns no executor,
// store or status writer: every effect goes through a port the caller wires.
package watch

// Purpose: the vocabulary of the watcher: its settings, a candidate head, the ports it calls and the
//          per-pass report.
// Inputs:  none (types only).
// Outputs: Config, Head, Scanner, Runner, Job, Outcome, Ports, PassReport, Skip.
// Constraints: no network import (architecture autoNoDirectIO); a port left nil takes the safe answer
//          named on its field.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// Config is the resolved watch.* keys plus the claim-ledger settings of this host.
type Config struct {
	Repos         []string      // owner/repo names to watch
	Interval      time.Duration // pass cadence (providers.github.poll_interval or --interval)
	Branches      []string      // watch.branches: extra branch names or globs
	MaxBranches   int           // watch.max_branches
	BranchDays    int           // watch.branch_days
	RetryMax      int           // watch.retry_max: retries of an error result
	RetryGap      time.Duration // watch.retry_gap
	StatusContext string        // watch.status_context: overrides the resolved result context
	Drain         time.Duration // watch.drain_sec: how long SIGTERM waits for a running attempt
	Host          string        // this host's station name
	Incarnation   string        // this process's incarnation (a claim names both)
	Hosts         []string      // the watch hosts of the fleet; the first sorted one is preferred
	Preferred     string        // an explicit preferred host
	Trusted       []string      // creators whose statuses count (the station login first)
	Step          time.Duration // how long a non-preferred host waits before it takes a commit
	RunLimit      time.Duration // a claim with no status for RunLimit+claims.DeadGrace is dead
}

// Head is one candidate commit found by the scan.
type Head struct {
	Repo      string
	SHA       string
	Branch    string // branch heads; "" for a pull request
	Default   bool
	Protected bool
	Extra     bool // selected by watch.branches
	PR        int
	Fork      bool // a pull request from another (or an unknown) repository
	// Trust is the TRUST source class of the head (owner, collaborator or untrusted).
	Trust       model.TrustClass
	CommittedAt time.Time
	Facts       model.TriggerFacts // kind api, source watch, auth per-repo
}

// Scan is one repository's discovery result.
type Scan struct {
	Heads       []Head
	NotModified bool // every conditional read answered 304: the previous heads stand
}

// QuotaError stops a pass: the provider refused for quota until RetryAt.
type QuotaError struct{ RetryAt time.Time }

func (q *QuotaError) Error() string {
	return "watch: quota reached, retry at " + q.RetryAt.UTC().Format(time.RFC3339)
}

// Scanner discovers the candidate heads of one repository (the HOST-17 head scan).
type Scanner interface {
	Scan(ctx context.Context, repo string) (Scan, error)
}

// Job is one attempt the runner executes.
type Job struct {
	Head    Head
	Context string   // the context the result goes to
	Only    []string // the residual check ids of a hosted use; empty means the whole pipeline
	// Isolated asks for an isolated runner (a collaborator head); the runner must refuse otherwise.
	Isolated bool
}

// Outcome is the state an attempt ended with.
type Outcome struct {
	State       report.State // success, failure or error
	Description string
	Cancelled   bool // the attempt was cancelled by the drain
}

// Runner runs one commit through the coordinator path (Options.Record set, trigger watch).
type Runner interface {
	Run(ctx context.Context, j Job) (Outcome, error)
}

// Hosted decides whether the repository's own hosted run stands (P7-AUTO-07).
type Hosted interface {
	Decide(ctx context.Context, repo, sha string, pushed time.Time) hostedtier.Result
}

// Ports are the watcher's effects. Ledger, Reporter, Scanner and Runner are required.
type Ports struct {
	Scanner  Scanner
	Ledger   report.Ledger   // claims and the status history (P7-AUTO-10)
	Reporter report.Reporter // the one status writer
	Runner   Runner
	// ResultContext resolves the result context of a repository (report.ResolveResultContext).
	// nil: the advisory nself-ci/watch, as for an unreadable protection.
	ResultContext func(ctx context.Context, owner, repo, override string) string
	Hosted        Hosted                                                // nil: always hand over
	Ready         func(ctx context.Context) (bool, []string)            // nil: not ready
	Isolated      func(ctx context.Context) bool                        // nil: no isolated runner
	TeamOwned     func(ctx context.Context, repo string) bool           // nil: personal
	Alert         func(ctx context.Context, h Head, description string) // the notifier failure alert
	Idle          func(ctx context.Context) (bool, error)               // one due background pipeline
	Publish       func(doc demand.Document) error                       // the demand document
	Heartbeat     func(at time.Time) error
	Log           func(event string, fields map[string]any)
	Now           func() time.Time                       // nil: time.Now
	After         func(d time.Duration) <-chan time.Time // nil: time.After
	Jitter        func() float64                         // in [0,1); nil: no random part
}

// Skip reasons a pass reports for a head it did not run.
const (
	SkipFinal     = "final"
	SkipClaimed   = "claimed"
	SkipWait      = "wait"
	SkipHosted    = "hosted_wait"
	SkipHostedUse = "hosted_use"
	SkipFork      = "fork"
	SkipTeam      = "team_owned"
	SkipIsolated  = "needs isolated runner"
	SkipApproval  = "needs approval"
	SkipNotReady  = "not_ready"
	SkipBusy      = "busy" // this pass already ran a commit
	SkipLost      = "lost"
	SkipError     = "ledger_error"
)

// Decision is what a pass did with one head.
type Decision struct {
	Repo    string `json:"repo"`
	SHA     string `json:"sha"`
	Action  string `json:"action"` // run, skip, post
	Reason  string `json:"reason,omitempty"`
	Code    string `json:"code,omitempty"`
	Context string `json:"context,omitempty"`
	State   string `json:"state,omitempty"`
}

// PassReport is one pass.
type PassReport struct {
	At         time.Time  `json:"at"`
	Ready      bool       `json:"ready"`
	ReadyWhy   []string   `json:"ready_why"`
	Decisions  []Decision `json:"decisions"`
	Ran        int        `json:"ran"`
	Queued     int        `json:"queued"`
	Held       int        `json:"held"`
	Duplicates int        `json:"duplicates"`
	QuotaUntil time.Time  `json:"quota_until,omitzero"`
	Idle       bool       `json:"idle_started"`
	Errors     []string   `json:"errors"`
}
