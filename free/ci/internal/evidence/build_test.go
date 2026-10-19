package evidence

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func runDoc(t *testing.T, dir string, st *store.Store, res cirun.Result) model.Evidence {
	t.Helper()
	row, found, err := st.PipelineByID(context.Background(), res.RunID)
	if err != nil || !found {
		t.Fatalf("pipeline row: %v %v", found, err)
	}
	docs, err := ForRun(context.Background(), st, row)
	if err != nil || len(docs) != 1 {
		t.Fatalf("stored run evidence: %d %v", len(docs), err)
	}
	return docs[0].Evidence
}

func TestBuilderBindsInputsAndRepo(t *testing.T) {
	dir, sha := newRepo(t, baseFiles)
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("true")), cirun.Hooks{})
	ev := runDoc(t, dir, st, res)
	want, err := InputsAt(context.Background(), nil, dir, sha)
	ok(t, err)
	if ev.Revision != sha || ev.Binding.State != "bound" || ev.WorktreeState != "clean" || ev.TrackedChangesDigest != nil {
		t.Fatalf("binding: %+v", ev)
	}
	if got := attest.Fresh(ev, want); got.State != "current" {
		t.Fatalf("a fresh run reads %+v", got)
	}
	if len(ev.InputDigests.Lockfiles) != 1 || ev.InputDigests.Lockfiles[0].Path != "go.sum" {
		t.Fatalf("lockfiles: %+v", ev.InputDigests.Lockfiles)
	}
	repo, found := RepoID(context.Background(), nil, dir)
	if !found || ev.RepoID != repo || ev.ToolVersions["git"] == "" {
		t.Fatalf("repo %q tools %v", ev.RepoID, ev.ToolVersions)
	}
	if _, err = attest.Verify(mustJSON(t, ev)); err != nil {
		t.Fatalf("the stored document does not verify: %v", err)
	}
}

func TestDirtyRunIsUnbound(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	write(t, dir, "a.txt", "changed\n")
	write(t, dir, "new.txt", "untracked\n")
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("true")), cirun.Hooks{})
	ev := runDoc(t, dir, st, res)
	if ev.Binding.State != "unbound" || ev.WorktreeState != "dirty" || ev.Binding.Reason == nil || *ev.Binding.Reason != "dirty" {
		t.Fatalf("a dirty tree must be unbound: %+v", ev.Binding)
	}
	diff, _ := git(context.Background(), nil, dir, "diff", "HEAD", "--binary")
	if ev.TrackedChangesDigest == nil || *ev.TrackedChangesDigest != digestString(diff) || diff == "" {
		t.Fatalf("tracked digest %v is not the sha256 of `git diff HEAD --binary`", ev.TrackedChangesDigest)
	}
	if ev.Freshness.State == "current" {
		t.Fatalf("unbound evidence is never current: %+v", ev.Freshness)
	}
	if reasons := attest.Consistent(ev); len(reasons) != 0 {
		t.Fatalf("inconsistent: %v", reasons)
	}
}

func TestJobThatRewritesALockfileUnbindsTheRun(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("sh", "-c", "echo tampered >> "+filepath.Join(dir, "go.sum"))), cirun.Hooks{})
	ev := runDoc(t, dir, st, res)
	if ev.Binding.State != "unbound" || ev.TrackedChangesDigest == nil {
		t.Fatalf("a run that changed the tree must be unbound: %+v", ev)
	}
}

func TestAttemptDocuments(t *testing.T) {
	dir, sha := newRepo(t, baseFiles)
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("true")), cirun.Hooks{})
	rows, err := st.EvidenceByRevision(context.Background(), sha, mustRow(t, st, res.RunID).InputDigest)
	ok(t, err)
	var attempts int
	for _, r := range rows {
		if r.AttemptID == "" {
			continue
		}
		attempts++
		ev, err := attest.Verify(r.Body)
		if err != nil || ev.Job == nil || *ev.Job != "unit" || ev.Attempt == nil || ev.Attempt.ID != r.AttemptID || len(ev.Checks) != 1 {
			t.Fatalf("attempt document: %+v %v", ev, err)
		}
	}
	if attempts != 1 {
		t.Fatalf("%d attempt documents, want 1", attempts)
	}
}

func mustRow(t *testing.T, st *store.Store, id string) store.PipelineRow {
	t.Helper()
	row, found, err := st.PipelineByID(context.Background(), id)
	if err != nil || !found {
		t.Fatalf("row %s: %v %v", id, found, err)
	}
	return row
}

// The selection of the plan is copied into the evidence unchanged (affected
// selection), and equals what the pipeline row holds.
func TestSelectionCopy(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	p := buildPlan(t, jobsOf("true"))
	p.Selection = model.Selection{Mode: "affected", Reason: "selection.affected", Digest: "sel-0123456789"}
	res, st := runPipeline(t, dir, t.TempDir(), p, cirun.Hooks{})
	ev := runDoc(t, dir, st, res)
	row := mustRow(t, st, res.RunID)
	if ev.Selection != p.Selection {
		t.Fatalf("selection %+v, plan %+v", ev.Selection, p.Selection)
	}
	if string(ev.Selection.Mode) != row.SelectionMode || string(ev.Selection.Reason) != row.SelectionReason || ev.Selection.Digest != row.SelectionDigest {
		t.Fatalf("evidence %+v differs from the pipeline row %+v", ev.Selection, row)
	}
}

// placement_ref is null with no hook and the registered value otherwise; an
// attempt the hook does not know stays null.
func TestPlacementRef(t *testing.T) {
	saved := cirun.PlacementRef
	t.Cleanup(func() { cirun.PlacementRef = saved })
	for name, tc := range map[string]struct {
		hook func(context.Context, string) (string, bool)
		want string
	}{
		"no hook":    {nil, ""},
		"not placed": {func(context.Context, string) (string, bool) { return "node-1", false }, ""},
		"placed":     {func(context.Context, string) (string, bool) { return "node-1/slot-2", true }, "node-1/slot-2"},
	} {
		t.Run(name, func(t *testing.T) {
			cirun.PlacementRef = tc.hook
			dir, sha := newRepo(t, baseFiles)
			res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("true")), cirun.Hooks{})
			ev := runDoc(t, dir, st, res)
			check := func(label string, got *string) {
				switch {
				case tc.want == "" && got != nil:
					t.Errorf("%s placement_ref = %q, want null", label, *got)
				case tc.want != "" && (got == nil || *got != tc.want):
					t.Errorf("%s placement_ref = %v, want %q", label, got, tc.want)
				}
			}
			check("run", ev.PlacementRef)
			rows, err := st.EvidenceByRevision(context.Background(), sha, mustRow(t, st, res.RunID).InputDigest)
			ok(t, err)
			for _, r := range rows {
				if r.AttemptID != "" {
					a, err := attest.Verify(r.Body)
					ok(t, err)
					check("attempt", a.PlacementRef)
				}
			}
		})
	}
}

type fixedPolicy string

func (p fixedPolicy) Resolve(context.Context, cirun.PolicyRequest) (cirun.PolicyResult, error) {
	return cirun.PolicyResult{Digest: string(p)}, nil
}

func TestPolicyDigest(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("true")), cirun.Hooks{Policy: fixedPolicy("pol-digest-123")})
	ev := runDoc(t, dir, st, res)
	row := mustRow(t, st, res.RunID)
	if ev.PolicyDigest != "pol-digest-123" || ev.PolicyDigest != row.PolicyDigest {
		t.Fatalf("policy_digest %q, row %q", ev.PolicyDigest, row.PolicyDigest)
	}
}

// --job runs one job: a check, never the required pipeline.
func TestJobRunIsAdhocEvidence(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	home := t.TempDir()
	c := &cirun.Coordinator{Opts: cirun.Options{Root: dir, Home: home, NoStatus: true, Job: "unit"}, Plan: buildPlan(t, jobsOf("true")), ConfigDigest: "cfg",
		Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	ok(t, err)
	if k := res.Summary.Doc.Pipeline.Kind; k != "adhoc" {
		t.Fatalf("a --job run is kind %q, want adhoc", k)
	}
}

// An edit that git was told to hide, made while the jobs run, still unbinds.
func TestHiddenEditDuringTheRunUnbinds(t *testing.T) {
	dir, _ := newRepo(t, baseFiles)
	script := "git -C " + dir + " update-index --skip-worktree a.txt && echo tampered >> " + filepath.Join(dir, "a.txt")
	res, st := runPipeline(t, dir, t.TempDir(), buildPlan(t, jobsOf("sh", "-c", script)), cirun.Hooks{})
	ev := runDoc(t, dir, st, res)
	if ev.Binding.State != "unbound" || ev.WorktreeState != "dirty" {
		t.Fatalf("a hidden edit made during the run left the evidence %s/%s", ev.Binding.State, ev.WorktreeState)
	}
}

// D-1098: an attempt document of an infrastructure-only failure reads `error`,
// the same as its run, not `fail`.
func TestAttemptDocumentOfATimeoutIsError(t *testing.T) {
	run := model.Evidence{
		Result: "error",
		Checks: []model.Check{{ID: "unit", JobID: "unit", Required: true, Substantive: true, Result: "error", FailureClass: "timeout"}},
	}
	doc := AttemptDocument(run, cirun.JobOutcome{ID: "unit", AttemptID: "a1", N: 1})
	if doc.Result != "error" {
		t.Fatalf("attempt document result %q, want error", doc.Result)
	}
}
