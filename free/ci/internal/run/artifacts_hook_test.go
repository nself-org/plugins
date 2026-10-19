package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

// artRig is newRig with a floor the test volume always clears. These tests
// change process-wide hook state, so none of them runs in parallel.
func artRig(t *testing.T, jobs map[string]model.Job) *rig {
	t.Helper()
	t.Setenv("NSELF_CI_STORE_MIN_FREE", "1")
	return newRig(t, jobs, gitFake{})
}

func artJob(script string, decls ...model.ArtifactDecl) model.Job {
	j := sh(script)
	j.Artifacts = decls
	return j
}

func committedRecords(t *testing.T, r *rig) []records.Record {
	t.Helper()
	recs, err := r.store(t).ArtifactList(context.Background(), records.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func attemptState(t *testing.T, r *rig, id string) string {
	t.Helper()
	state, _, err := r.store(t).AttemptStateEpoch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return string(state)
}

func TestArtifactsHookCommitsOutputs(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": artJob(`mkdir -p out && echo app > out/app && echo ok > report.txt`,
		model.ArtifactDecl{Path: "out/app", Kind: "binary"}, model.ArtifactDecl{Path: "report.txt", Name: "test-report", Kind: "report", Visibility: "restricted"})})
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StatePassed || res.Exit != 0 {
		t.Fatalf("build: %+v\n%s", o, r.Out)
	}
	recs := committedRecords(t, r)
	if len(recs) != 2 {
		t.Fatalf("records: %+v", recs)
	}
	byName := map[string]records.Record{}
	for _, rc := range recs {
		byName[rc.Name] = rc
	}
	app, rep := byName["out/app"], byName["test-report"]
	if app.Kind != records.KindBinary || app.Size != 4 || app.State != records.StateCommitted || app.Project != r.C.Opts.Root || app.Revision != testSHA || app.Isolation != "process" || app.Producer != "nself-ci/build" || app.Platform == "" {
		t.Fatalf("app: %+v", app)
	}
	if rep.Kind != records.KindReport || rep.Visibility != authz.VisibilityRestricted || rep.ExpiresAt == nil {
		t.Fatalf("report: %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(r.Home, "work", app.Attempt)); !os.IsNotExist(err) {
		t.Fatalf("workspace left behind: %v", err)
	}
}

func TestArtifactsHookMissingOutputFailsInfra(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": artJob(`echo built`, model.ArtifactDecl{Path: "out/app"})})
	res := r.run(t)
	o := outcome(res, "build")
	if o.State != StateFailed || o.Code != "E712" || o.FailureClass != "infra" || res.Exit == 0 {
		t.Fatalf("build: %+v", o)
	}
	if got := attemptState(t, r, o.AttemptID); got != "failed" {
		t.Fatalf("attempt state %s", got)
	}
	if len(committedRecords(t, r)) != 0 {
		t.Fatal("a record was committed")
	}
}

func TestArtifactsHookRefusedDeclarationNeverRunsTheJob(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": artJob(`touch ran`, model.ArtifactDecl{Path: "../../etc/passwd"})})
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StateFailed || o.Code != "E683" {
		t.Fatalf("build: %+v", o)
	}
	if _, err := os.Stat(filepath.Join(r.C.Opts.Root, "ran")); err == nil {
		t.Fatal("the job ran")
	}
}

// TestArtifactsHookUncommittedBlocksPass: with the finalize seam removed the
// records stay staged, and the store's guard still refuses the pass.
func TestArtifactsHookUncommittedBlocksPass(t *testing.T) {
	saved := Registered.AttemptFinalizes
	Registered.AttemptFinalizes = nil
	t.Cleanup(func() { Registered.AttemptFinalizes = saved })
	r := artRig(t, map[string]model.Job{"build": artJob(`echo data > out.txt`, model.ArtifactDecl{Path: "out.txt"})})
	res := r.run(t)
	o := outcome(res, "build")
	if o.State != StateFailed || o.FailureClass != "infra" || res.Exit == 0 || !strings.Contains(o.Detail, "not committed") {
		t.Fatalf("build: %+v", o)
	}
	if got := attemptState(t, r, o.AttemptID); got != "failed" {
		t.Fatalf("attempt state %s", got)
	}
	staged, err := r.store(t).ArtifactStaged(context.Background(), o.AttemptID)
	if err != nil || len(staged) != 0 {
		t.Fatalf("a failed attempt keeps no records: %+v %v", staged, err)
	}
	if len(committedRecords(t, r)) != 0 {
		t.Fatal("an uncommitted record is visible")
	}
}

func TestArtifactsHookSecretFailsTheAttempt(t *testing.T) {
	red := redact.New()
	const secret = "registered-secret-value-9988776655"
	if err := red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	saved := artifactsOn.scanner
	artifactsOn.scanner = func() authz.ValueScanner { return red }
	t.Cleanup(func() { artifactsOn.scanner = saved })
	r := artRig(t, map[string]model.Job{"build": artJob(`printf 'key=`+secret+`' > leak.txt`, model.ArtifactDecl{Path: "leak.txt"})})
	res := r.run(t)
	o := outcome(res, "build")
	if o.State != StateFailed || o.Code != "E712" || o.FailureClass != "infra" || strings.Contains(o.Detail, secret) {
		t.Fatalf("build: %+v", o)
	}
	if blobs, _ := filepath.Glob(filepath.Join(r.Home, "blobs", "artifacts-*", "sha256", "*", "*")); len(blobs) != 0 {
		t.Fatalf("stored %v", blobs)
	}
	if len(committedRecords(t, r)) != 0 || strings.Contains(r.Out.String(), secret) {
		t.Fatal("record committed or secret printed")
	}
}

func TestArtifactsHookWorkdir(t *testing.T) {
	j := artJob(`echo w > pkg.tgz`, model.ArtifactDecl{Path: "pkg.tgz", Kind: "package"})
	j.Workdir = "sub"
	r := artRig(t, map[string]model.Job{"build": j})
	if err := os.MkdirAll(filepath.Join(r.C.Opts.Root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StatePassed {
		t.Fatalf("build: %+v\n%s", o, r.Out)
	}
	if recs := committedRecords(t, r); len(recs) != 1 || recs[0].Name != "pkg.tgz" || recs[0].Kind != records.KindPackage {
		t.Fatalf("records: %+v", recs)
	}
}

func TestArtifactsHookBadCapConfigFailsTheAttempt(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": artJob(`echo x > a`, model.ArtifactDecl{Path: "a"})})
	t.Setenv("NSELF_CI_ARTIFACT_MAX_FILE", "lots")
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StateFailed || o.Code != "E712" || !strings.Contains(o.Detail, "NSELF_CI_ARTIFACT_MAX_FILE") {
		t.Fatalf("build: %+v", o)
	}
}

func TestArtifactsHookJobWithoutArtifactsCostsNothing(t *testing.T) {
	r := artRig(t, map[string]model.Job{"a": sh("true")})
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("exit %d", res.Exit)
	}
	if len(committedRecords(t, r)) != 0 {
		t.Fatal("records for a job with no outputs")
	}
}

func TestArtifactsHookImplicitDeclarations(t *testing.T) {
	records.RegisterImplicit(func(j model.Job) []model.ArtifactDecl {
		if j.Kind != "sbom-test" {
			return nil
		}
		return []model.ArtifactDecl{{Path: "sbom.json", Kind: "sbom"}}
	})
	j := sh(`echo '{}' > sbom.json`)
	j.Kind = "sbom-test"
	r := artRig(t, map[string]model.Job{"s": j})
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	if recs := committedRecords(t, r); len(recs) != 1 || recs[0].Kind != records.KindSBOM {
		t.Fatalf("records: %+v", recs)
	}
}

func TestArtifactsHookPrunesExpiredAtFirstAttempt(t *testing.T) {
	clock := time.Now()
	saved := artifactsOn.now
	artifactsOn.now = func() time.Time { return clock }
	t.Cleanup(func() { artifactsOn.now = saved })
	r := artRig(t, map[string]model.Job{"build": artJob(`echo v1 > a.txt`, model.ArtifactDecl{Path: "a.txt", Retention: "1d"})})
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("first run: %s", r.Out)
	}
	if len(committedRecords(t, r)) != 1 {
		t.Fatal("first record")
	}
	// a later run in a fresh process (new coordinator, same home) prunes the expired record at its first attempt
	clock = clock.Add(48 * time.Hour)
	artifactsOn.pruned.Delete(r.Home)
	r2 := artRig(t, map[string]model.Job{"other": artJob(`echo v2 > b.txt`, model.ArtifactDecl{Path: "b.txt", Retention: "keep"})})
	r2.C.Opts.Home = r.Home
	if res := r2.run(t); res.Exit != 0 {
		t.Fatalf("second run: %s", r2.Out)
	}
	recs, err := r.store(t).ArtifactList(context.Background(), records.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "b.txt" {
		t.Fatalf("after the auto prune: %+v", recs)
	}
}

func TestWorkdirRoot(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "real", "deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(ws, "real"), filepath.Join(ws, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pinned, err := os.OpenRoot(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pinned.Close() }()
	got, closeFn, err := workdirRoot(pinned, "")
	if err != nil || got != pinned {
		t.Fatalf("empty: %v", err)
	}
	closeFn()
	if _, err = pinned.Stat("."); err != nil {
		t.Fatal("closing the workspace itself must be a no-op")
	}
	sub, closeSub, err := workdirRoot(pinned, "real/deep")
	if err != nil || sub == pinned {
		t.Fatalf("nested: %v", err)
	}
	closeSub()
	for _, bad := range []string{"..", "../x", "real/../../x", "missing", "link", "file"} {
		if _, _, err := workdirRoot(pinned, bad); err == nil || !strings.HasPrefix(err.Error(), "E712") {
			t.Fatalf("workdir %q: %v", bad, err)
		}
	}
}

func TestArtifactsHookWithoutStoreIsInert(t *testing.T) {
	a := AttemptInfo{AttemptID: "x", Job: model.Job{Artifacts: []model.ArtifactDecl{{Path: "p"}}}}
	if artifactsOn.AttemptStart(context.Background(), a) != nil || artifactsOn.AfterJob(context.Background(), a, nil) != nil || artifactsOn.AttemptFinalize(context.Background(), a, 1) != nil {
		t.Fatal("a hook without a store must do nothing")
	}
}

func TestArtifactsHookDiskFloorRefusesBeforeAnyByteLands(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": artJob(`echo data > out.txt`, model.ArtifactDecl{Path: "out.txt"})})
	t.Setenv("NSELF_CI_STORE_MIN_FREE", "100%") // the floor is the whole volume: nothing can be written
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StateFailed || o.Code != "E712" || !strings.Contains(o.Detail, "disk_floor") {
		t.Fatalf("build: %+v", o)
	}
	if blobs, _ := filepath.Glob(filepath.Join(r.Home, "blobs", "artifacts-*", "sha256", "*", "*")); len(blobs) != 0 {
		t.Fatalf("stored %v", blobs)
	}
}

func TestArtifactsHookRefusesASecretInADeclaration(t *testing.T) {
	red := redact.New()
	const secret = "declared-secret-value-55443322"
	if err := red.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	saved := artifactsOn.scanner
	artifactsOn.scanner = func() authz.ValueScanner { return red }
	t.Cleanup(func() { artifactsOn.scanner = saved })
	r := artRig(t, map[string]model.Job{"build": artJob(`touch ran`, model.ArtifactDecl{Path: "out.txt", Name: "n-" + secret})})
	o := outcome(r.run(t), "build")
	if o.State != StateFailed || o.Code != "E712" || strings.Contains(o.Detail, secret) {
		t.Fatalf("build: %+v", o)
	}
	if _, err := os.Stat(filepath.Join(r.C.Opts.Root, "ran")); err == nil {
		t.Fatal("the job ran")
	}
}

// A callback from an earlier incarnation of the attempt keeps the epoch it
// launched with and is refused (E711) instead of adopting the new one.
func TestArtifactsHookStaleCallbackIsFenced(t *testing.T) {
	r := artRig(t, map[string]model.Job{"build": sh("true")})
	st := r.store(t)
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pstale", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "jstale", PipelineID: "pstale", Name: "stale", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: "stale", JobID: "jstale", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if err := storetest.Admit(ctx, st, "stale"); err != nil {
		t.Fatal(err)
	}
	if err := st.Transition(ctx, "stale", "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
	job := model.Job{Artifacts: []model.ArtifactDecl{{Path: "out.txt"}}}
	old := AttemptInfo{AttemptID: "stale", JobName: "stale", Job: job, Store: st, Home: r.Home, Epoch: 2}
	if err := artifactsOn.AttemptStart(ctx, old); err != nil {
		t.Fatal(err)
	}
	// the attempt row runs again: running -> lost -> retryable -> queued -> leased -> running
	for _, s := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"running", "lost", 2}, {"lost", "retryable", 3}, {"retryable", "queued", 4}, {"queued", "leased", 5}, {"leased", "running", 6}} {
		d := store.TransitionDetail{Epoch: s.epoch}
		d.Operator = s.from == "lost" || s.from == "retryable"
		var err error
		if s.to == "leased" {
			err = storetest.Admit(ctx, st, "stale") // a bare queued -> leased is refused
		} else {
			err = st.Transition(ctx, "stale", s.from, s.to, d)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	fresh := old
	fresh.Epoch = 7
	if err := artifactsOn.AttemptStart(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "out.txt"), []byte("old workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = artifactsOn.AfterJob(ctx, old, root) // the old incarnation's callback, resumed late
	if err == nil || !strings.HasPrefix(err.Error(), "E711") {
		t.Fatalf("stale callback: %v", err)
	}
	if staged, _ := st.ArtifactStaged(ctx, "stale"); len(staged) != 0 {
		t.Fatalf("stale bytes were staged: %+v", staged)
	}
	if err = artifactsOn.AfterJob(ctx, fresh, root); err != nil {
		t.Fatalf("the current incarnation stages: %v", err)
	}
}

// A job that declares nothing never reads the store from these hooks, so a
// store error cannot fail it.
func TestArtifactsHookJobWithoutDeclarationsNeverTouchesTheStore(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close() // every read now fails
	a := AttemptInfo{AttemptID: "none", JobName: "none", Job: model.Job{}, Store: st, Epoch: 2}
	ctx := context.Background()
	if err := artifactsOn.AttemptStart(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := artifactsOn.AfterJob(ctx, a, nil); err != nil {
		t.Fatal(err)
	}
	if err := artifactsOn.AttemptFinalize(ctx, a, 3); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactsHookAutoPruneRunsAgainAfterTheInterval(t *testing.T) {
	clock := time.Now()
	h := &artifactsHook{scanner: artifactsOn.scanner, env: func(string) string { return "1" }, now: func() time.Time { return clock }, warn: func(string) {}}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	home := t.TempDir()
	a := AttemptInfo{Store: st, Home: home}
	h.autoPrune(context.Background(), a)
	first, _ := h.pruned.Load(home)
	clock = clock.Add(pruneEvery - time.Minute)
	h.autoPrune(context.Background(), a)
	if again, _ := h.pruned.Load(home); again != first {
		t.Fatal("pruned again inside the interval")
	}
	clock = clock.Add(2 * time.Minute)
	h.autoPrune(context.Background(), a)
	if later, _ := h.pruned.Load(home); later == first {
		t.Fatal("not pruned again after the interval")
	}
	var warned []string
	h.warn = func(s string) { warned = append(warned, s) }
	_ = st.Close()
	clock = clock.Add(2 * pruneEvery)
	h.autoPrune(context.Background(), a)
	if len(warned) != 1 || !strings.Contains(warned[0], "auto prune") {
		t.Fatalf("a failing auto prune must warn: %v", warned)
	}
}
