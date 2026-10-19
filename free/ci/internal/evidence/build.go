package evidence

// Purpose: complete the coordinator-recorded evidence of a run (P7-CI-38):
// the input digests and the binding as the run began, the repo id, tool
// versions, the placement reference, and one evidence document per attempt.
// Inputs:  run.EvidenceFacts and the base document the coordinator built.
// Outputs: the final model.Evidence; per-attempt documents are recorded
// through store.RecordEvidence.
// Constraints: digests come only from internal/attest; the begin snapshot is
// read before any job starts and is never refreshed (a job that rewrites a
// lockfile makes the tree dirty, which unbinds the run); an error leaves the
// base document, which has no input digests and so never reads as current;
// evidence carries no caller-specific field and no secret.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// snapshot is what Begin read before the first job.
type snapshot struct {
	inputs    model.InputDigests
	inputsErr error
	tree      Worktree
	repoID    string
	git       string
}

// Builder implements run.EvidenceBuilder.
type Builder struct{}

func init() { cirun.RegisterEvidence(Builder{}) }

// Begin reads the inputs and the binding of the checkout.
func (Builder) Begin(ctx context.Context, root string, rev cirun.Revision, c cirun.Capture) any {
	s := &snapshot{tree: ReadWorktree(ctx, c, root), git: GitVersion(ctx, c, root)}
	if rev.SHA == "" {
		return s
	}
	s.inputs, s.inputsErr = InputsAt(ctx, c, root, rev.SHA)
	if id, ok := RepoID(ctx, c, root); ok {
		s.repoID = id
	}
	return s
}

// Complete returns the final run document.
func (Builder) Complete(ctx context.Context, begin any, f cirun.EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	s, _ := begin.(*snapshot)
	if s == nil {
		s = &snapshot{inputsErr: fail(CodeWorktree, 2, "the inputs were not read before the run")}
	}
	if s.inputsErr == nil && f.Rev.SHA != "" {
		ev.InputDigests = s.inputs
	}
	tree := s.tree
	if end := ReadWorktree(ctx, f.Git, f.Root); end.Dirty && !tree.Dirty { // edits made while the jobs ran, hidden or not
		tree = end
	}
	if f.Job != "" { // one job is a check, not the required pipeline: it can never gate a revision
		ev.Pipeline.Kind = "adhoc"
	}
	Apply(&ev, tree, f.Rev.Dirty || tree.Dirty || f.Rev.SHA == "")
	ev.Freshness = model.Freshness{State: "current"}
	if ev.Binding.State != "bound" { // D2: unbound evidence is never current
		why := "the worktree is not the named commit"
		ev.Freshness = model.Freshness{State: "unknown", Reason: &why}
	}
	if s.repoID != "" {
		ev.RepoID = s.repoID
	}
	if s.git != "" {
		ev.ToolVersions["git"] = s.git
	}
	ev.PlacementRef = commonPlacement(ctx, f.Jobs)
	if f.Store != nil {
		row, _, _ := f.Store.PipelineByID(ctx, f.RunID) // attempts are filed under the digest the run row carries
		for _, o := range f.Jobs {
			if o.AttemptID == "" || o.State == cirun.StateSkipped {
				continue
			}
			doc := AttemptDocument(ev, o)
			body, err := marshal(doc)
			if err != nil {
				return ev, err
			}
			if err = f.Store.RecordEvidence(ctx, "ev-"+o.AttemptID, f.RunID, o.AttemptID, f.Rev.SHA, row.InputDigest, body); err != nil {
				return ev, err
			}
		}
	}
	return ev, nil
}

// AttemptDocument narrows a run document to one attempt: its job, its check,
// the verdict of that check alone, and its own placement.
func AttemptDocument(run model.Evidence, o cirun.JobOutcome) model.Evidence {
	ev := run
	job := o.ID
	ev.Job, ev.Attempt = &job, &model.AttemptRef{ID: o.AttemptID, N: max(o.N, 1)}
	ev.Checks, ev.Counts = nil, model.Counts{}
	for _, c := range run.Checks {
		if c.JobID != o.ID {
			continue
		}
		ev.Checks = append(ev.Checks, c)
		ev.Counts = tally(ev.Checks)
	}
	ev.Result = model.Verdict(ev.Checks)
	if run.Result == "cancelled" {
		ev.Result = "cancelled"
	}
	ev.PlacementRef = nil
	if ref, ok := placement(context.Background(), o.AttemptID); ok {
		ev.PlacementRef = &ref
	}
	return ev
}

func tally(checks []model.Check) model.Counts {
	var c model.Counts
	for _, k := range checks {
		switch k.Result {
		case "pass":
			c.Pass++
		case "fail":
			c.Fail++
		case "skip":
			c.Skip++
		case "error":
			c.Error++
		}
	}
	c.Total = len(checks)
	return c
}

// placement reads the registered placement hook; no hook, or not ok, is nil.
func placement(ctx context.Context, attemptID string) (string, bool) {
	if cirun.PlacementRef == nil || attemptID == "" {
		return "", false
	}
	ref, ok := cirun.PlacementRef(ctx, attemptID)
	return ref, ok && ref != ""
}

// commonPlacement is the placement every placed attempt shares. Attempts on
// different nodes have no single placement: the field stays null and the
// per-attempt documents carry each one.
func commonPlacement(ctx context.Context, jobs []cirun.JobOutcome) *string {
	seen := map[string]bool{}
	for _, o := range jobs {
		if ref, ok := placement(ctx, o.AttemptID); ok {
			seen[ref] = true
		}
	}
	if len(seen) != 1 {
		return nil
	}
	refs := make([]string, 0, 1)
	for r := range seen {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	return &refs[0]
}
