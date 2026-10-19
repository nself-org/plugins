package plan

// Purpose: the input and output values of the pure planner (P7-CI-35).
// Inputs:  Input = config + detector facts + planned catalogue checks +
// revision context + local world. Every field is a value the caller probed;
// the planner reads no file, process, clock or network.
// Outputs: Plan, which marshals to the `data` of `nself ci plan --json`.
// Constraints: deterministic: the same Input yields byte-identical JSON.

import (
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Check is one catalogue check applied to a directory (a gates.PlannedCheck
// with its {target} already substituted into Argv).
type Check struct {
	ID, Kind, Tool, Workdir, Target string
	Argv                            []string
	Required, Substantive           bool
	Path                            model.JobPath
	// Platform limits the check to one platform (matrix) and Requires lists
	// the placement tools it needs (requires.tools), e.g. darwin/arm64 and
	// xcode for xcode-test (P7-CI-64).
	Platform string
	Requires []string
}

// Revision is the git and trigger context of the plan.
type Revision struct {
	Branch        string
	DefaultBranch bool     // the revision is on the default branch
	Release       bool     // the revision is a release (tag)
	BaseKnown     bool     // Changed is a real diff against a base
	Changed       []string // repo-relative, slash-separated, sorted by the caller
	Env           []string // names of environment variables that are set
}

// World is the local runner world, probed by the caller.
type World struct {
	Platform      string   `json:"platform"`           // host, "linux/amd64"
	Tools         []string `json:"tools,omitempty"`    // executables on PATH
	Docker        bool     `json:"docker"`             // a usable Docker daemon
	Services      []string `json:"services,omitempty"` // services the host can start
	ServicesKnown bool     `json:"-"`                  // Services was probed
	Labels        []string `json:"labels,omitempty"`
	Accelerators  []string `json:"accelerators,omitempty"`
	CPU           int      `json:"cpu,omitempty"`
	MemMB         int      `json:"mem_mb,omitempty"`
	Runners       []string `json:"runners,omitempty"` // platforms of other registered runners
	Remote        bool     `json:"remote"`            // nodes or providers exist; the scheduler checks requirements
}

// Input is everything Build reads.
type Input struct {
	Config   model.PipelineConfig
	Facts    []detect.Fact
	Checks   []Check
	Revision Revision
	World    World
	Pipeline string        // --pipeline NAME
	Trigger  model.Trigger // --trigger; "" means local
}

// Job states in the plan.
const (
	StatePlanned     = "planned"
	StateSkip        = "skip"        // runs as a skip check (when.env_missing)
	StateUnplaceable = "unplaceable" // no runner can run it (E618 at run time)
)

// PlannedJob is one expanded job: a matrix entry of a configured job or a
// catalogue check.
type PlannedJob struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Platform   string       `json:"platform,omitempty"`
	Check      string       `json:"check,omitempty"`
	Layer      int          `json:"layer"`
	State      string       `json:"state"`
	Reason     model.Reason `json:"reason"`            // why the job is in the plan
	Blocked    model.Reason `json:"blocked,omitempty"` // why it cannot run normally
	Detail     string       `json:"detail,omitempty"`
	Deps       []string     `json:"deps"`
	Dependants []string     `json:"dependants"`
	Job        model.Job    `json:"job"`
}

// Skipped is a job that is not in the plan, with the reason.
type Skipped struct {
	Job    string       `json:"job"`
	Reason model.Reason `json:"reason"`
	Detail string       `json:"detail,omitempty"`
}

// Plan is the validated pipeline.
type Plan struct {
	Pipeline  model.PipelineRef `json:"pipeline"`
	Trigger   model.Trigger     `json:"trigger"`
	Selection model.Selection   `json:"selection"`
	Support   []string          `json:"support"`
	FailFast  string            `json:"fail_fast"`
	Layers    [][]string        `json:"layers"`
	Jobs      []PlannedJob      `json:"jobs"`
	Skipped   []Skipped         `json:"skipped"`
	Placement any               `json:"placement,omitempty"`
	// Cache is the resolved top-level `cache` block of the config the plan
	// was built from; it is not part of the plan document.
	Cache *model.CacheConfig `json:"-"`
}

// Fail-fast scopes.
const (
	FailFastDependants = "dependants"
	FailFastPipeline   = "pipeline"
)
