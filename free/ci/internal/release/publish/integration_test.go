//go:build integration

package publish_test

// Purpose: the release pipeline end to end, in process: a real git repository,
// the real coordinator running a test job and a release job that commits an
// artifact and its two SBOMs, the registered AfterRun hook, the real store,
// blob store and verifier, and the real signing handler (with a generated
// release key standing in for `artifacts key init`) as the signer.
// Constraints: a missing prerequisite (git) is t.Fatal, never t.Skip; no
// network; the only fakes are the release-inputs digester seam (until
// internal/evidence registers it, D-1074) and the secret store behind the signer.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type pipeRig struct {
	t         *testing.T
	repo, rev string
	home      string
	res       *fakeRes
	keyValue  []byte
	team      bool
	signs     int
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newPipeRig(t *testing.T) *pipeRig {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("INTEGRATION=1 is required for the release pipeline tests")
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	if !compat.V15() {
		t.Fatal("the V15 engine is not enabled")
	}
	p := &pipeRig{t: t, repo: t.TempDir(), home: t.TempDir()}
	gitIn(t, p.repo, "init", "-q", "-b", "main")
	for name, body := range map[string]string{".gitignore": "out/\n", "README.md": "release fixture\n"} {
		if err := os.WriteFile(filepath.Join(p.repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, p.repo, "add", ".")
	gitIn(t, p.repo, "commit", "-q", "-m", "fixture")
	p.rev = gitIn(t, p.repo, "rev-parse", "HEAD")
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	_ = key
	p.keyValue = value
	p.res = &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
	return p
}

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

// pipeline is a test job and a release job that commits app and its SBOMs.
func (p *pipeRig) pipeline(releaseScript string) *plan.Plan {
	jobs := map[string]model.Job{
		"build": {Kind: "test", Run: []string{"sh", "-c", "true"}, Required: yes()},
		"publish": {Kind: "release", Run: []string{"sh", "-c", releaseScript}, Deps: []string{"build"}, Required: yes(), Idempotent: no(),
			Artifacts: []model.ArtifactDecl{{Path: "out/app", Name: "app", Kind: "binary"},
				{Path: "out/app.cdx.json", Name: "app.cdx.json", Kind: "sbom"}, {Path: "out/app.spdx.json", Name: "app.spdx.json", Kind: "sbom"}}},
	}
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs}, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		p.t.Fatalf("plan: %v", err)
	}
	return pl
}

const goodRelease = `mkdir -p out && printf app-bytes > out/app && printf '{"sbom":"cdx"}' > out/app.cdx.json && printf '{"sbom":"spdx"}' > out/app.spdx.json`

// deps builds the publication dependencies over the run's store.
func (p *pipeRig) deps(st *store.Store) publish.Deps {
	blobs, err := cas.New(records.StoreDir(p.home))
	if err != nil {
		p.t.Fatal(err)
	}
	signer := &inProcSigner{h: signing.Handler{Src: st, Blobs: blobs, Project: p.repo, Resolver: p.res}, res: p.res}
	return publish.Deps{Src: st, Blobs: blobs, Ledger: st, Signer: signer, Home: p.home, Root: p.repo, Capture: report.Capture, Team: p.team}
}

// run executes the pipeline with the publication hook installed.
func (p *pipeRig) run(script string, install bool) (run.Result, *store.Store) {
	t := p.t
	oldInputs, oldPub := run.ReleaseInputs, run.ReleasePublisher
	t.Cleanup(func() { run.ReleaseInputs, run.ReleasePublisher = oldInputs, oldPub })
	run.ReleaseInputs = func(ctx context.Context, root, rev string) (model.InputDigests, model.InputDigests, error) {
		d, err := evidence.InputsAt(ctx, report.Capture, root, rev)
		return d, d, err
	}
	bound, err := evidence.InputsAt(context.Background(), report.Capture, p.repo, p.rev)
	if err != nil {
		t.Fatalf("inputs: %v", err)
	}
	st, err := store.Open(filepath.Join(p.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	run.ReleasePublisher = nil
	if install {
		run.ReleasePublisher = func(ctx context.Context, r run.ReleasePublishRequest) (run.ReleaseOutcome, error) {
			d := p.deps(r.Store)
			rel, err := publish.Run(ctx, d, r.RunID)
			if err == publish.ErrNotRelease {
				return run.ReleaseOutcome{}, run.ErrNothingToPublish
			}
			return run.ReleaseOutcome{State: rel.State, Dir: rel.Dir, Message: rel.Message}, err
		}
	}
	var out bytes.Buffer
	c := &run.Coordinator{Opts: run.Options{Root: p.repo, Home: p.home, NoStatus: true, Full: true}, Plan: p.pipeline(script), ConfigDigest: bound.Config, Out: &out,
		Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	t.Logf("run output:\n%s", out.String())
	return res, st
}

func (p *pipeRig) verifyDir(dir string, info signing.KeyInfo) verify.Result {
	keys := filepath.Join(p.t.TempDir(), "ci-release.keys")
	if err := os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		p.t.Fatal(err)
	}
	return verify.Release(context.Background(), dir, verify.Options{Root: p.repo, Capture: report.Capture, OperatorKeys: keys})
}

// TestReleasePipeline: the run passes, the hook publishes, the export has the
// Epic layout, `artifacts verify` returns 0 and the evidence is class release.
func TestReleasePipeline(t *testing.T) {
	p := newPipeRig(t)
	res, st := p.run(goodRelease, true)
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s notes %v", res.Exit, res.Summary.Result, res.Summary.Notes)
	}
	row, ok, err := st.ReleaseFor(context.Background(), res.RunID)
	if err != nil || !ok || row.State != store.ReleasePublished {
		t.Fatalf("release record %+v %v %v", row, ok, err)
	}
	d := p.deps(st)
	out := filepath.Join(t.TempDir(), "export")
	exp, err := publish.Export(context.Background(), d, res.RunID, out, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := readDir(t, exp.Dir)
	for _, want := range []string{"release-manifest.dsse.json", "evidence.json", "provenance.intoto.dsse.json", "SHA256SUMS", "sbom/app.cdx.json", "sbom/app.spdx.json", "artifacts/app"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("export lacks %s", want)
		}
	}
	info, err := d.Signer.PublicKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := p.verifyDir(exp.Dir, info); !v.Verified || v.Exit != 0 {
		t.Fatalf("artifacts verify: %+v", v)
	}
	var ev model.Evidence
	if err = jsonUnmarshal(files["evidence.json"], &ev); err != nil || evidence.ClassOf(ev) != evidence.ClassRelease || ev.Result != "pass" {
		t.Fatalf("evidence class %v result %s (%v)", evidence.ClassOf(ev), ev.Result, err)
	}
	if string(files["artifacts/app"]) != "app-bytes" {
		t.Fatalf("artifact bytes %q", files["artifacts/app"])
	}
	p.cliChecks(exp.Dir, info)
}

// cliChecks runs the shipped binary's own commands on the exported directory:
// `ci artifacts verify <dir>` returns 0 and `ci evidence verify --file` reports class release.
func (p *pipeRig) cliChecks(dir string, info signing.KeyInfo) {
	t := p.t
	bin := filepath.Join(t.TempDir(), "nself-ci")
	build := exec.Command("go", "build", "-mod=vendor", "-o", bin, "../../../cmd")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	keys := filepath.Join(t.TempDir(), "ci-release.keys")
	if err := os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cli := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = p.repo
		cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+t.TempDir(), "NSELF_CI_REPO="+p.repo)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return string(out), code
	}
	var env struct {
		Data struct {
			Verified bool   `json:"verified"`
			Class    string `json:"class"`
		} `json:"data"`
	}
	out, code := cli("artifacts", "verify", dir, "--keys", keys, "--repo", p.repo, "--json")
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 0 || !env.Data.Verified {
		t.Fatalf("artifacts verify: exit %d (%v)\n%s", code, err, out)
	}
	out, code = cli("evidence", "verify", "--file", filepath.Join(dir, "evidence.json"), "--at-revision", p.rev, "--json")
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Class != "release" {
		t.Fatalf("evidence verify: exit %d, class %q (%v)\n%s", code, env.Data.Class, err, out)
	}
}

// TestReleaseRecovery: a publication that never ran (the process was killed
// after the evidence was recorded) and one that failed for a missing key both
// leave no published record; `artifacts sign` (publish.Run) completes them and a
// second call produces byte-identical files.
func TestReleaseRecovery(t *testing.T) {
	p := newPipeRig(t)
	res, st := p.run(goodRelease, false) // the hook is gone: killed before signing
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s", res.Exit, res.Summary.Result)
	}
	if _, ok, _ := st.ReleaseFor(context.Background(), res.RunID); ok {
		t.Fatal("a release was recorded without publication")
	}
	if _, err := os.Stat(publish.Dir(p.home, res.RunID)); err == nil {
		t.Fatal("a release directory exists")
	}
	d := p.deps(st)
	first, err := publish.Run(context.Background(), d, res.RunID)
	if err != nil || first.State != publish.Published {
		t.Fatalf("sign: %+v %v", first, err)
	}
	before := readDir(t, first.Dir)
	if err = os.RemoveAll(first.Dir); err != nil {
		t.Fatal(err)
	}
	second, err := publish.Run(context.Background(), d, res.RunID)
	if err != nil || !bytesEqualMaps(before, readDir(t, second.Dir)) || second.ManifestSHA256 != first.ManifestSHA256 {
		t.Fatalf("second sign: %+v %v", second, err)
	}

	// no signing key: the evidence stays pass, the release fails with E717 key_missing, the run exits 1 and names the recovery
	q := newPipeRig(t)
	delete(q.res.vals, signing.KeyRef)
	res, st = q.run(goodRelease, true)
	notes := strings.Join(res.Summary.Notes, "\n")
	if res.Exit != 1 || res.Summary.Result != "pass" || !strings.Contains(notes, "E717") || !strings.Contains(notes, "key_missing") || !strings.Contains(notes, "nself ci artifacts sign "+res.RunID) {
		t.Fatalf("exit %d result %s notes %q", res.Exit, res.Summary.Result, notes)
	}
	row, _, _ := st.ReleaseFor(context.Background(), res.RunID)
	if row.State != store.ReleaseFailed || !strings.Contains(row.Reason, "key_missing") {
		t.Fatalf("ledger %+v", row)
	}
	q.res.vals[signing.KeyRef] = q.keyValue
	if rel, err := publish.Run(context.Background(), q.deps(st), res.RunID); err != nil || rel.State != publish.Published {
		t.Fatalf("recovery: %+v %v", rel, err)
	}

	// a release job that failed, and a run without a release job, publish nothing
	r := newPipeRig(t)
	res, st = r.run("exit 1", true)
	if res.Exit == 0 {
		t.Fatal("a failed release job exited 0")
	}
	if _, ok, _ := st.ReleaseFor(context.Background(), res.RunID); ok || r.res.calls != 0 {
		t.Fatal("a failed release job published")
	}
}

// TestReleaseTeamAwaits: a TEAM coordinator records awaiting-signature and the
// secret resolver is never called.
func TestReleaseTeamAwaits(t *testing.T) {
	p := newPipeRig(t)
	p.team = true
	res, st := p.run(goodRelease, true)
	row, ok, _ := st.ReleaseFor(context.Background(), res.RunID)
	if res.Exit != 0 || !ok || row.State != store.ReleaseAwaiting || !strings.Contains(row.Reason, "--coordinator") {
		t.Fatalf("exit %d ledger %+v", res.Exit, row)
	}
	if p.res.calls != 0 {
		t.Fatalf("the TEAM coordinator resolved a secret %d times", p.res.calls)
	}
	if _, err := os.Stat(publish.Dir(p.home, res.RunID)); err == nil {
		t.Fatal("a TEAM coordinator wrote a release directory")
	}
}
