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
	if err != nil || len(staged) != 1 || staged[0].State != records.StateStaged {
		t.Fatalf("staged: %+v %v", staged, err)
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

func TestWorkdirOf(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "real", "deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(ws, "real"), filepath.Join(ws, "link")); err != nil {
		t.Fatal(err)
	}
	if got, err := workdirOf(ws, ""); err != nil || got != ws {
		t.Fatalf("empty: %q %v", got, err)
	}
	resolved, _ := filepath.EvalSymlinks(ws)
	if got, err := workdirOf(resolved, "real/deep"); err != nil || got != filepath.Join(resolved, "real", "deep") {
		t.Fatalf("nested: %q %v", got, err)
	}
	for _, bad := range []string{"..", "../x", "real/../../x", "missing", "link"} {
		if _, err := workdirOf(resolved, bad); err == nil || !strings.HasPrefix(err.Error(), "E712") {
			t.Fatalf("workdir %q: %v", bad, err)
		}
	}
}

func TestArtifactsHookWithoutStoreIsInert(t *testing.T) {
	a := AttemptInfo{AttemptID: "x", Job: model.Job{Artifacts: []model.ArtifactDecl{{Path: "p"}}}}
	if artifactsOn.AttemptStart(context.Background(), a) != nil || artifactsOn.AfterJob(context.Background(), a, t.TempDir()) != nil || artifactsOn.AttemptFinalize(context.Background(), a, 1) != nil {
		t.Fatal("a hook without a store must do nothing")
	}
}
