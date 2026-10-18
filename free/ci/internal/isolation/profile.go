package isolation

// Purpose: the contract every isolation level implements behind the executor's Isolator hook.
// Inputs:  a Profile per level (container here, gVisor and Tart in later Tickets).
// Outputs: Availability facts, a wrapped exec.JobSpec and labelled-resource listing.
// Constraints: a profile builds argv and facts only; process start stays in internal/exec.
// A probe may lower the level a profile declares and never raise it (Epic D7, X5).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Availability is what a probe found. Level is the highest level the profile can
// deliver now ("" when none); Reason explains an empty or lowered level.
type Availability struct {
	Level  model.Isolation   `json:"level,omitempty"`
	Reason string            `json:"reason,omitempty"`
	Facts  map[string]string `json:"facts,omitempty"`
}

// Available reports whether any level can be delivered.
func (a Availability) Available() bool { return a.Level != "" }

// Attempt identifies the attempt a profile prepares or tears down.
type Attempt struct {
	ID, CoordinatorID string
	Slot              int
}

// Profile is one isolation level.
type Profile interface {
	// Level is the level the profile declares.
	Level() model.Isolation
	// Probe reports what the runner can deliver now.
	Probe(ctx context.Context) Availability
	// Prepare checks, before the job starts, that the profile still works.
	Prepare(ctx context.Context, a Attempt) error
	// Wrap returns the spec with the profile's confinement attached.
	Wrap(s exec.JobSpec) (exec.JobSpec, error)
	// Teardown removes what the profile created for the attempt; it is idempotent.
	Teardown(ctx context.Context, a Attempt) error
}

// Lister is implemented by profiles that label their resources; SCHED-09 lists
// leftovers by label.
type Lister interface {
	List(ctx context.Context, label string) ([]string, error)
}

// Effective returns the level a profile delivers: the lower of the declared level
// and the probed one. A probed level that is empty or unknown yields "".
func Effective(declared, probed model.Isolation) model.Isolation {
	d, p := trust.RankIsolation(declared), trust.RankIsolation(probed)
	if d < 0 || p < 0 {
		return ""
	}
	if p < d {
		return probed
	}
	return declared
}
