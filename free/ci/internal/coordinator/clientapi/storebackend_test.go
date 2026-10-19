package clientapi

// Purpose: tests of StoreBackend over a real store: a submitted trigger becomes a run id, the intake row
// is tied to its pipeline while it runs and after, projects come from the intake row, and the evidence
// comes back as the stored signed bytes.
// Inputs:  a SQLite store in t.TempDir(), a real Coordinator for Enqueue, a golden evidence document.
// Outputs: pass or fail.
// Constraints: no network; the pipeline rows are written through the store's own methods.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func goodEvidence(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../attest/testdata/adversarial/good.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Evidence json.RawMessage `json:"evidence"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if _, err = attest.Verify(f.Evidence); err != nil {
		t.Fatalf("fixture does not verify: %v", err)
	}
	return f.Evidence
}

func TestStoreBackendLifecycle(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	c, err := coordinator.New(coordinator.Config{Store: st, PolicyDigest: "t"})
	if err != nil {
		t.Fatal(err)
	}
	b := &StoreBackend{Store: st, Home: t.TempDir(), Enqueue: c.Enqueue}
	sub, err := b.Submit(ctx, SubmitRequest{Project: "acme/app", Revision: goodRev}, "token:x:mia")
	if err != nil || !strings.HasPrefix(sub.ID, "in-") || sub.Outcome != "enqueued" {
		t.Fatalf("submit: %+v %v", sub, err)
	}
	if again, _ := b.Submit(ctx, SubmitRequest{Project: "acme/app", Revision: goodRev}, "token:x:mia"); again.ID != sub.ID || again.Outcome == "enqueued" {
		t.Fatalf("a repeated submit must coalesce into the same run: %+v", again)
	}
	it, ok, err := b.Get(ctx, sub.ID)
	if err != nil || !ok || it.Status != "queued" || it.Project != "acme/app" || it.ID != sub.ID {
		t.Fatalf("queued: %+v %v %v", it, ok, err)
	}
	if _, err = b.Cancel(ctx, sub.ID, "x"); ReasonOf(err) != ReasonConflict {
		t.Fatalf("a run that has not started cannot be cancelled: %v", err)
	}
	if _, ok, _ = b.Get(ctx, "in-missing"); ok {
		t.Fatal("an unknown intake id must not be found")
	}
	row, claimed, err := st.IntakeClaim(ctx, "coord-1")
	if err != nil || !claimed || row.ID != sub.ID {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if it, _, _ = b.Get(ctx, sub.ID); it.Status != "running" {
		t.Fatalf("dispatched without a pipeline reads running: %+v", it)
	}
	pid := "run-" + goodRev[:8]
	p := store.PipelineRow{ID: pid, Project: "/tmp/checkout-1", Revision: goodRev, Trigger: "api", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-1", PipelineID: pid, Name: "build", Kind: "test", InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if it, _, _ = b.Get(ctx, sub.ID); it.ID != pid || it.Project != "acme/app" || len(it.Jobs) != 1 {
		t.Fatalf("a running intake row must resolve to its pipeline: %+v", it)
	}
	if it, ok, _ = b.Get(ctx, pid); !ok || it.Project != "acme/app" {
		t.Fatalf("a running pipeline must already belong to the project of its intake row: %+v", it)
	}
	if err = st.IntakeFinish(ctx, sub.ID, "coord-1", pid, "ok", true); err != nil {
		t.Fatal(err)
	}
	if err = st.FinishPipeline(ctx, pid, "passed"); err != nil {
		t.Fatal(err)
	}
	loose := store.PipelineRow{ID: "run-loose", Project: "/home/op/repo", Revision: strings.Repeat("b", 40), Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "passed"}
	if err = st.CreatePipeline(ctx, loose); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{pid, pid[:8], sub.ID, goodRev, "latest"} {
		if it, ok, err = b.Get(ctx, ref); err != nil || !ok || it.Project == "" && ref != "latest" {
			t.Fatalf("get %q: %+v %v %v", ref, it, ok, err)
		}
	}
	list, err := b.List(ctx, 50)
	if err != nil || len(list) != 2 || list[0].ID != pid || list[0].Project != "acme/app" || list[1].ID != "run-loose" || list[1].Project != "" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err = b.Evidence(ctx, pid); ReasonOf(err) != ReasonNotFound {
		t.Fatalf("no evidence yet must be not_found: %v", err)
	}
	doc := goodEvidence(t)
	if err = st.RecordRunEvidence(ctx, "ev-1", pid, goodRev, "i", doc); err != nil {
		t.Fatal(err)
	}
	got, err := b.Evidence(ctx, pid)
	if err != nil || string(got) != string(doc) {
		t.Fatalf("evidence must come back byte for byte: %v", err)
	}
	if n, err := b.Cancel(ctx, pid, "x"); err != nil || n != 0 {
		t.Fatalf("cancel of a finished run asks for nothing: %d %v", n, err)
	}
	if ok, err := b.Logs(ctx, LogQuery{Run: pid, Job: "build"}, nil); ok || err != nil {
		t.Fatalf("a job with no attempt has no logs: %v %v", ok, err)
	}
}

func TestInflightProjectRefusesAmbiguity(t *testing.T) {
	row := func(project, state, pipeline string) store.IntakeRow {
		return store.IntakeRow{Project: project, Revision: goodRev, State: state, PipelineID: pipeline}
	}
	for name, c := range map[string]struct {
		rows []store.IntakeRow
		want string
	}{
		"one dispatched row":              {[]store.IntakeRow{row("acme/app", store.IntakeDispatched, "")}, "acme/app"},
		"two rows, one project":           {[]store.IntakeRow{row("acme/app", store.IntakeDispatched, ""), row("acme/app", store.IntakeDispatched, "")}, "acme/app"},
		"same commit in two projects":     {[]store.IntakeRow{row("acme/app", store.IntakeDispatched, ""), row("fork/app", store.IntakeDispatched, "")}, ""},
		"a row that already names a run":  {[]store.IntakeRow{row("acme/app", store.IntakeDispatched, "run-9")}, ""},
		"a queued row is not running yet": {[]store.IntakeRow{row("acme/app", store.IntakeQueued, "")}, ""},
		"none":                            {nil, ""},
	} {
		if got := inflightProject(c.rows, goodRev); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	if got := inflightProject([]store.IntakeRow{row("acme/app", store.IntakeDispatched, "")}, "other"); got != "" {
		t.Errorf("a different revision matched: %q", got)
	}
}

// An ended pipeline that no intake row names must not take the project of a dispatched row with the same
// revision (List and Get follow one rule).
func TestListLeavesEndedUnownedPipelinesUnowned(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	c, _ := coordinator.New(coordinator.Config{Store: st, PolicyDigest: "t"})
	b := &StoreBackend{Store: st, Home: t.TempDir(), Enqueue: c.Enqueue}
	if _, err = b.Submit(ctx, SubmitRequest{Project: "other/proj", Revision: goodRev}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.IntakeClaim(ctx, "coord-1"); !ok {
		t.Fatal("nothing to claim")
	}
	old := store.PipelineRow{ID: "run-old", Project: "/home/op/repo", Revision: goodRev, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "passed"}
	if err = st.CreatePipeline(ctx, old); err != nil {
		t.Fatal(err)
	}
	list, err := b.List(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list {
		if it.ID == "run-old" && it.Project != "" {
			t.Fatalf("an ended, unclaimed pipeline was given project %q", it.Project)
		}
	}
	if it, ok, _ := b.Get(ctx, "run-old"); !ok || it.Project != "" {
		t.Fatalf("get: %+v %v", it, ok)
	}
}

// Two projects whose dispatched rows wait on the same commit (a fork has the same SHA): neither may be
// tied to the running pipeline, or one project's members would see and cancel the other's run.
func TestSameCommitInTwoProjectsIsNeverCrossed(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	c, _ := coordinator.New(coordinator.Config{Store: st, PolicyDigest: "t"})
	b := &StoreBackend{Store: st, Home: t.TempDir(), Enqueue: c.Enqueue}
	var ids []string
	for _, proj := range []string{"acme/app", "fork/app"} {
		sub, err := b.Submit(ctx, SubmitRequest{Project: proj, Revision: goodRev}, "x")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sub.ID)
		if _, ok, _ := st.IntakeClaim(ctx, "coord-"+proj); !ok {
			t.Fatal("nothing to claim")
		}
	}
	run := store.PipelineRow{ID: "run-shared", Project: "/tmp/checkout", Revision: goodRev, Trigger: "api", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-s", PipelineID: "run-shared", Name: "build", Kind: "test", InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		it, ok, err := b.Get(ctx, id)
		if err != nil || !ok || it.ID != id || len(it.Jobs) != 0 || it.Status != "running" {
			t.Fatalf("row %d must stay an unattached running run, got %+v (%v %v)", i, it, ok, err)
		}
	}
	it, ok, _ := b.Get(ctx, "run-shared")
	if !ok || it.Project != "" {
		t.Fatalf("the shared pipeline belongs to nobody until its dispatch finishes: %+v", it)
	}
	list, _ := b.List(ctx, 20)
	for _, it := range list {
		if it.ID == "run-shared" && it.Project != "" {
			t.Fatalf("List attributed the shared pipeline to %q", it.Project)
		}
	}
}
