package hostedtier

// Purpose: the vocabulary of the hosted tier (P7-AUTO-07): the three decisions, their reasons, the
//          operator-scope settings (hosted.mode, hosted.queued_sec, hosted.running_sec) and the
//          read-only ports the observer needs.
// Inputs:  none (types only).
// Outputs: Decision, Result, Config, Funnel, Breaker, CheckRun.
// Constraints: reads only; nothing here dispatches a workflow or writes a status (HOST-03 and the
//          Checks adapter own those).

import (
	"context"
	"time"
)

// Decision is what the auto tier does with the repository's own hosted workflow result.
type Decision string

const (
	Use      Decision = "use"       // the hosted result stands (a red result stays red)
	Wait     Decision = "wait"      // the hosted run is still inside its timers
	HandOver Decision = "hand_over" // local nodes run the gates
)

// Reasons are stable machine strings; E798 is the code of an unreadable hosted state.
const (
	ReasonSuccess     = "hosted.success"
	ReasonFailure     = "hosted.failure"
	ReasonQueued      = "hosted.queued"
	ReasonQueuedLong  = "hosted.queued_timeout"
	ReasonRunning     = "hosted.running"
	ReasonRunningLong = "hosted.running_timeout"
	ReasonAbsent      = "hosted.absent"
	ReasonAbsentLong  = "hosted.absent_timeout"
	ReasonCancelled   = "hosted.cancelled"
	ReasonBilling     = "hosted.billing_blocked"
	ReasonAPIError    = "hosted.api_error"
	ReasonUnavailable = "hosted.unavailable"
	ReasonOff         = "hosted.off"
	NotePaidDenied    = "hosted.paid_needs_allowed"
	CodeUnreadable    = "E798"
)

// GitHubActionsAppID is the app that owns the Actions check runs (CI D18); a run of another app
// with the same name, such as the Checks adapter's own, is never the hosted result.
const GitHubActionsAppID int64 = 15368

// Defaults of the operator-scope keys.
const (
	DefaultContext    = "nself-ci"
	DefaultQueued     = 30 * time.Second
	DefaultRunning    = 4 * time.Minute
	DefaultAbsent     = 30 * time.Second
	DefaultResidualID = "secrets.gitleaks"
)

// Mode is the operator-scope key hosted.mode.
type Mode string

const (
	ModeOff  Mode = "off"
	ModeFree Mode = "free"
	ModePaid Mode = "paid"
)

// Config is the resolved hosted.* keys. Zero fields take the defaults through Normalize.
type Config struct {
	Mode     Mode
	Context  string
	AppID    int64
	Queued   time.Duration
	Running  time.Duration
	Absent   time.Duration
	Residual []string
	// PaidAllowed is the answer of schedpolicy.PaidAllowed for the hosted provider; paid needs it.
	PaidAllowed bool
}

// CheckRun is the part of a GitHub check run the decision reads.
type CheckRun struct {
	ID                   int64
	Name                 string
	AppID                int64
	Status, Conclusion   string
	CreatedAt, StartedAt time.Time
}

// Funnel is the read-only view of the HOST-18 REST funnel for one repository.
type Funnel interface {
	CheckRuns(ctx context.Context, sha string) ([]CheckRun, error)
	Annotations(ctx context.Context, runID int64) ([]string, error)
}

// Breaker is the HOST-11 breaker of the hosted provider.
type Breaker interface {
	Blocked(ctx context.Context) bool
	MarkBlocked(until time.Time)
}

// Result is one decision.
type Result struct {
	Decision Decision
	Reason   string
	Mode     Mode     // the effective mode
	Notes    []string // for example NotePaidDenied
	Residual []string // for Use: the checks the workflow does not cover
	Requests int      // API requests made, for tests and the usage ledger
}
