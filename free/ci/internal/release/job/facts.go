package job

// Purpose: turn what the coordinator knows about a running pipeline into the
// Facts the admission step judges: a coordinator-recorded evidence document of
// the release job's deps, plus the protected identity of the run.
// Inputs:  RunState, read by internal/run/release_hook.go from the run's store
// rows, the git worktree and the inputs digester.
// Outputs: Facts. Pure: no store, git or file access here.
// Constraints: the evidence is always coordinator-recorded and carries only
// checks for deps that have an outcome row, so a missing dep shows as
// deps_jobs_mismatch, never as a pass; a dirty or moved worktree is unbound; a
// check passes only for a job whose status is "passed".

import (
	"runtime"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Dep is the stored outcome of one job the release job depends on.
type Dep struct {
	Name     string
	Status   string // the job row status: passed, failed, skipped, cancelled, ...
	Required bool
}

// RunState is the input of BuildFacts.
type RunState struct {
	RunID, RepoID, Revision, PolicyDigest, ConfigDigest string
	SourceTrust                                         model.TrustClass
	Trigger                                             model.Trigger
	Pipeline                                            model.PipelineRef
	Selection                                           model.Selection
	// Deps names the release job's deps; Outcomes holds a row for each one that exists.
	Deps     []string
	Outcomes []Dep
	// Clean says the worktree is clean and at Revision.
	Clean bool
	// Bound holds the inputs at Revision, Now the inputs in the worktree; nil
	// means no digester answered (freshness is then unknown, which refuses).
	Bound, Now *model.InputDigests
}

// BuildFacts assembles the Facts of one release attempt.
func BuildFacts(s RunState) Facts {
	byName := make(map[string]Dep, len(s.Outcomes))
	for _, o := range s.Outcomes {
		byName[o.Name] = o
	}
	var checks []model.Check
	for _, name := range s.Deps {
		o, ok := byName[name]
		if !ok {
			continue
		}
		result := model.CheckResult("skip")
		switch strings.ToLower(o.Status) {
		case "passed":
			result = "pass"
		case "failed":
			result = "fail"
		}
		checks = append(checks, model.Check{ID: name, JobID: name, Kind: "job", Tool: "nself-ci", Required: true,
			Substantive: true, Result: result})
	}
	inputs := model.InputDigests{Config: s.ConfigDigest, Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}}
	if s.Bound != nil {
		inputs = *s.Bound
	}
	ev := model.Evidence{Schema: "ci.evidence/v1", Revision: s.Revision, RepoID: s.RepoID, RunID: s.RunID, Pipeline: s.Pipeline,
		Trigger: s.Trigger, SourceTrust: s.SourceTrust, PolicyDigest: s.PolicyDigest, InputDigests: inputs,
		Runner:   model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, Signature: model.Signature{Kind: "coordinator-recorded"},
		ToolVersions: map[string]string{}, Checks: checks, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: s.Selection}
	if s.Clean {
		ev.WorktreeState, ev.Binding = "clean", model.Binding{State: "bound"}
	} else {
		reason := model.BindingReason("dirty")
		ev.WorktreeState, ev.Binding = "dirty", model.Binding{State: "unbound", Reason: &reason}
	}
	for _, c := range checks {
		switch c.Result {
		case "pass":
			ev.Counts.Pass++
		case "fail":
			ev.Counts.Fail++
		default:
			ev.Counts.Skip++
		}
	}
	ev.Counts.Total = len(checks)
	ev.Result = model.Verdict(checks)
	return Facts{Evidence: ev, RepoID: s.RepoID, Revision: s.Revision, PolicyDigest: s.PolicyDigest, Deps: s.Deps, Current: s.Now}
}
