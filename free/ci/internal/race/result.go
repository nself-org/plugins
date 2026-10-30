package race

// Purpose: what a race records: the result of one sibling, whether it is
// conclusive, and the one evidence document the pipeline keeps (CI-40).
// Inputs:  sibling results as the attempt states and failure classes give them.
// Outputs: Evidence naming the winner, the cancelled and superseded siblings,
// and any later conclusive result that disagrees with the winner (flaky).
// Constraints: a flaky entry is data only: it never changes Evidence.Result.

// Attempt states and failure classes a sibling ends with.
const (
	StatePassed    = "passed"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
	ClassCode      = "code"
)

// Dispositions of a sibling that did not win.
const (
	Cancelled  = "cancelled"  // ended by the race's cancel
	Superseded = "superseded" // finished after the winner with the same or no conclusive result
	Flaky      = "flaky"      // finished after the winner with a different conclusive result
	Lost       = "lost"       // ended without a conclusive result before any winner
)

// Sibling is one admitted attempt of a raced job.
type Sibling struct {
	N         int    `json:"n"`
	AttemptID string `json:"attempt_id"`
	LeaseID   string `json:"lease_id,omitempty"`
	RunnerID  string `json:"runner_id"`
	Epoch     int64  `json:"epoch"`
}

// Result is how a sibling ended.
type Result struct {
	State        string `json:"state"`
	FailureClass string `json:"failure_class,omitempty"`
	ExitCode     int    `json:"exit_code"`
}

// Conclusive says r decides the job: a pass, or a code-class failure (a red
// result is conclusive). infra, error, timeout, lost and cancelled are not.
func Conclusive(r Result) bool {
	return r.State == StatePassed || r.State == StateFailed && r.FailureClass == ClassCode
}

// Loser is a sibling that did not win, with how it ended.
type Loser struct {
	Sibling
	Disposition string  `json:"disposition"`
	Result      *Result `json:"result,omitempty"` // nil when it never reported
}

// Evidence is the one document a race records and posts.
type Evidence struct {
	Job      string            `json:"job"`
	Width    int               `json:"width"`
	Winner   *Sibling          `json:"winner,omitempty"` // nil when no sibling was conclusive
	Result   Result            `json:"result"`
	Losers   []Loser           `json:"losers"`
	Flaky    bool              `json:"flaky"` // a later sibling disagreed; Result is unchanged
	Excluded map[string]string `json:"excluded,omitempty"`
	Notes    []string          `json:"notes,omitempty"`
}

// CancelledSiblings lists the attempts the race cancelled.
func (e Evidence) CancelledSiblings() []string {
	var out []string
	for _, l := range e.Losers {
		if l.Disposition == Cancelled {
			out = append(out, l.AttemptID)
		}
	}
	return out
}
