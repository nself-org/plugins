package run

// Purpose: the base ci.evidence/v1 document of a run of one job, for a caller outside this package that
// runs the job itself (the agent: P7-NODE-23). It is buildEvidence, so a single job gets the same check
// result, verdict, counts, diagnosis and excerpt bound as a coordinator-recorded run (D-1004).
// Inputs:  one job outcome, its start and end, and the identity the caller owns (run id, revision,
//          pipeline, trigger, source trust, runner, producer).
// Outputs: a model.Evidence whose binding is bound, for the caller to complete or unbind.
// Constraints: no I/O; the revision is the caller's, so a caller that cannot vouch for it unbinds the result.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// SingleJobInput is what SingleJobEvidence assembles a one-job document from.
type SingleJobInput struct {
	RunID, Revision, RepoID string
	Pipeline                model.PipelineRef
	Trigger                 model.Trigger
	Source                  string // the source trust class of the run
	Selection               model.Selection
	Runner                  model.RunnerRef
	Producer                model.Producer
	Job                     JobOutcome
	Start, End              time.Time
}

// SingleJobEvidence builds the document of a run of in.Job alone.
func SingleJobEvidence(in SingleJobInput) model.Evidence {
	// buildEvidence fails only when the document cannot be marshaled for its digest, which this one can always be.
	e, _, _ := buildEvidence(evidenceInput{RunID: in.RunID, Revision: in.Revision, Source: in.Source, Pipeline: in.Pipeline, Selection: in.Selection,
		Trigger: in.Trigger, Jobs: []JobOutcome{in.Job}, Cancelled: in.Job.State == StateCancelled, Start: in.Start, End: in.End})
	e.RepoID, e.Runner, e.Producer = in.RepoID, in.Runner, in.Producer
	return e
}
