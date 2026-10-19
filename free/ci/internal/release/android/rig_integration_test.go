//go:build integration || integration_android

package android

// Purpose: the shared rig of the Android lane's end-to-end tests: a real git
// repository holding a fake Android project, the real coordinator, the real
// store, blob store, signing handler, publication and verifier, and the real
// secret Source with the personal-node compat source. The fakes are the gradle
// wrapper (a committed script), the `nself` cli behind the secret Source (it
// answers E541, so the compat source supplies the values), the syft runner and
// the secret store behind the release signer.
// Constraints: a missing prerequisite (git, INTEGRATION=1) is t.Fatal, never
// t.Skip; no network; the planted secret values are obviously fake.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	_ "github.com/nself-org/plugins/free/ci/internal/events" // the shipped binary links the event and log sinks; so does the rig
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	_ "github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// planted values: what the sinks must never hold.
const (
	plantedPwOne  = "store-pw-PLANTED-5c1e77"
	plantedAlias  = "alias-PLANTED-b2d90a"
	plantedPwTwo  = "key-pw-PLANTED-e84f13"
	plantedKSMark = "FAKE-KEYSTORE-PLANTED-9f3a"
)

// gradlewScript checks the lane's contract (keystore decoded, all four values in
// the environment) and builds one APK; it prints the secrets so redaction is exercised.
func gradlewScript(marker string) string {
	return "#!/bin/sh\nset -eu\ntest \"${1:-}\" = assembleRelease\ntouch " + marker + "\ntest -s app/release.keystore\n" +
		"m=FAKE-KEYSTORE-; grep -q \"${m}PLANTED-9f3a\" app/release.keystore\n" + // the marker is spelled in two pieces: the repo must not hold a planted value
		"echo \"signing alias=$ANDROID_KEY_ALIAS store=$ANDROID_KEYSTORE_PASSWORD key=$ANDROID_KEY_PASSWORD\"\n" +
		"mkdir -p app/build/outputs/apk/release\nprintf fake-apk-bytes > app/build/outputs/apk/release/app-release.apk\n"
}

type rig struct {
	t         *testing.T
	repo, rev string
	home      string
	marker    string
	// sbomStep, when set, replaces the fake syft step (the laptop leg runs the pinned syft).
	sbomStep *SBOMStep
	res      *fakeRes
	keystore []byte
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

// newRig is the fake-gradle rig: a committed fake wrapper and the planted keystore environment.
func newRig(t *testing.T) *rig {
	t.Helper()
	p := &rig{t: t, marker: filepath.Join(t.TempDir(), "gradle-ran")}
	p.keystore = append([]byte(plantedKSMark), bytes.Repeat([]byte{0, 1, 2, 3, 250}, 40)...)
	p.setup(func(repo string) {
		writeFiles(t, repo, map[string]string{
			"android/gradlew": gradlewScript(p.marker),
			".gitignore":      "build/\n.nself-ci/\nrelease.keystore\n",
		})
	})
	// ignored leftovers of an earlier build in the operator's checkout: the workspace copy carries them
	writeFiles(t, p.repo, map[string]string{
		"android/app/build/outputs/apk/release/app-1.0-release.apk":   "stale-apk-from-v1.0",
		"android/.nself-ci/sbom/android/app-1.0-release.apk.cdx.json": "{}",
	})
	t.Setenv("ANDROID_KEYSTORE_BASE64", base64.StdEncoding.EncodeToString(p.keystore))
	t.Setenv("ANDROID_KEYSTORE_PASSWORD", plantedPwOne)
	t.Setenv("ANDROID_KEY_ALIAS", plantedAlias)
	t.Setenv("ANDROID_KEY_PASSWORD", plantedPwTwo)
	return p
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// setup makes the repository (the required job's config plus what prepare writes), its CI home
// and the release signing key.
func (p *rig) setup(prepare func(repo string)) {
	t := p.t
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	if !compat.V15() {
		t.Fatal("the V15 engine is not enabled")
	}
	p.repo, p.home = t.TempDir(), t.TempDir()
	writeFiles(t, p.repo, map[string]string{".nself/ci.yaml": "version: 1\njobs:\n  unit:\n    kind: test\n    run: [sh, -c, \"true\"]\n"})
	prepare(p.repo)
	gitIn(t, p.repo, "init", "-q", "-b", "main")
	gitIn(t, p.repo, "add", ".")
	gitIn(t, p.repo, "commit", "-q", "-m", "fixture")
	p.rev = gitIn(t, p.repo, "rev-parse", "HEAD")
	_, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	p.res = &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
}

// nselfStub is the cli behind the job's secret Source: the command exists and no secret is stored (E541).
type nselfStub struct{}

func (nselfStub) Run(_ context.Context, c secrets.Call) (secrets.Output, error) {
	if c.Argv[len(c.Argv)-1] == "--help" {
		return secrets.Output{Bytes: []byte("usage: resolve <ref>...\n")}, nil
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve", "error": map[string]string{"code": "E541", "message": "not set"}})
	return secrets.Output{Bytes: b, ExitCode: 1}, nil
}

// fakeRes is the secret store behind the release signer.
type fakeRes struct {
	vals  map[string][]byte
	calls int
}

func (r *fakeRes) Probe(context.Context) error { return nil }
func (r *fakeRes) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
	r.calls++
	var out []secrets.Value
	for _, ref := range refs {
		v, ok := r.vals[ref.Raw]
		if !ok {
			return nil, &secrets.Error{Code: "E676", Message: "not found", Cause: "E541"}
		}
		out = append(out, secrets.Value{Ref: ref, Bytes: append([]byte(nil), v...)})
	}
	return out, nil
}

// inProcSigner is the signer client over the real handler.
type inProcSigner struct {
	h   signing.Handler
	res *fakeRes
}

func (s *inProcSigner) Sign(ctx context.Context, runID string, items []signing.Item) ([]sdk.Envelope, error) {
	req := signing.Request{RunID: runID, Items: items}
	raw, _ := json.Marshal(req)
	_, payloads, err := signing.DecodeRequest(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.h.Handle(ctx, req, payloads)
}

func (s *inProcSigner) PublicKey(ctx context.Context) (signing.KeyInfo, error) {
	return signing.Public(ctx, s.res, signing.KeyAttempt)
}

func (p *rig) deps(st *store.Store) publish.Deps {
	blobs, err := cas.New(records.StoreDir(p.home))
	if err != nil {
		p.t.Fatal(err)
	}
	signer := &inProcSigner{h: signing.Handler{Src: st, Blobs: blobs, Project: p.repo, Resolver: p.res}, res: p.res}
	return publish.Deps{Src: st, Blobs: blobs, Ledger: st, Signer: signer, Home: p.home, Root: p.repo, Capture: report.Capture}
}

// lane resolves and plans the Android lane for the project in android/.
func (p *rig) lane() (*plan.Plan, string) {
	cfg := model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": {Kind: "test", Run: []string{"sh", "-c", "true"}}}}
	l, err := Resolve(Input{Config: cfg, Workdir: "android", Timeout: 5 * time.Minute, Required: []string{"unit"}, Env: EnvFrom(os.Getenv)})
	if err != nil || !l.Synthesized {
		p.t.Fatalf("lane %+v %v", l, err)
	}
	pl, err := plan.Build(plan.Input{Config: l.Config, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}, Pipeline: l.Pipeline})
	if err != nil {
		p.t.Fatalf("plan: %v", err)
	}
	return pl, l.Pipeline
}

// run executes the lane with the publication, SBOM and secret wiring the
// shipped command installs, and returns the result, the store and the output.
func (p *rig) run(rt run.SecretsRuntime) (run.Result, *store.Store, string) {
	t := p.t
	oldInputs, oldPub := run.ReleaseInputs, run.ReleasePublisher
	t.Cleanup(func() {
		run.ReleaseInputs, run.ReleasePublisher = oldInputs, oldPub
		run.ConfigureSecrets(run.SecretsRuntime{Mode: secrets.ModePersonalLocal})
	})
	run.ReleaseInputs = func(ctx context.Context, root, rev string) (model.InputDigests, model.InputDigests, error) {
		bound, err := evidence.InputsAt(ctx, report.Capture, root, rev)
		if err != nil {
			return bound, model.InputDigests{}, err
		}
		now, err := evidence.InputsOf(os.DirFS(root))
		return bound, now, err
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
	run.ReleasePublisher = func(ctx context.Context, r run.ReleasePublishRequest) (run.ReleaseOutcome, error) {
		rel, err := publish.Run(ctx, p.deps(r.Store), r.RunID)
		if err == publish.ErrNotRelease {
			return run.ReleaseOutcome{}, run.ErrNothingToPublish
		}
		return run.ReleaseOutcome{State: rel.State, Dir: rel.Dir, Message: rel.Message}, err
	}
	step := SBOMStep{Home: p.home, Pin: testPin(), NewRunner: func(string) sbom.Runner { return &fakeSyft{t: t} }}
	if p.sbomStep != nil {
		step = *p.sbomStep
	} else {
		installSyft(t, p.home)
	}
	t.Cleanup(Install(step))
	run.ConfigureSecrets(rt)
	pl, pipeline := p.lane()
	var out bytes.Buffer
	c := &run.Coordinator{Opts: run.Options{Root: p.repo, Home: p.home, Pipeline: pipeline, NoStatus: true, Full: true}, Plan: pl, ConfigDigest: bound.Config, Out: &out,
		Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	t.Logf("run output:\n%s", out.String())
	return res, st, out.String()
}

// personal is the operator's own machine: the real secret Source over the cli stub and the process environment.
func personal() run.SecretsRuntime {
	return run.SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: nselfStub{}}
}

func (p *rig) verifyDir(dir string, info signing.KeyInfo) verify.Result {
	keys := filepath.Join(p.t.TempDir(), "ci-release.keys")
	if err := os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		p.t.Fatal(err)
	}
	return verify.Release(context.Background(), dir, verify.Options{Root: p.repo, Capture: report.Capture, OperatorKeys: keys})
}

func needIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("INTEGRATION=1 is required for the Android lane tests")
	}
}

// sinkHits lists every planted value found in the files under roots and in the extra texts.
func sinkHits(t *testing.T, planted []string, texts map[string]string, roots ...string) []string {
	t.Helper()
	var hits []string
	scan := func(where string, b []byte) {
		for _, v := range planted {
			if bytes.Contains(b, []byte(v)) {
				hits = append(hits, where+": "+v[:min(len(v), 12)])
			}
		}
	}
	files := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			files++
			scan(path, b)
			if filepath.Base(path) == "release.keystore" {
				hits = append(hits, path+": a keystore file was left behind")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range texts {
		scan(name, []byte(text))
	}
	if files < 5 { // a positive control: the walk really read the state, the blobs and the release
		t.Fatalf("the sink walk read only %d files", files)
	}
	return hits
}

// logLine returns the first line containing want in any file under dir ("" when none).
func logLine(t *testing.T, dir, want string) string {
	t.Helper()
	found := ""
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" || !d.Type().IsRegular() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if i := bytes.Index(b, []byte(want)); i >= 0 {
			j := bytes.IndexByte(b[i:], '\n')
			if j < 0 {
				j = len(b) - i
			}
			found = string(b[i : i+j])
		}
		return nil
	})
	return found
}

// cliVerify runs the shipped binary's `artifacts verify` on the exported release: it returns 0.
func (p *rig) cliVerify(dir string, info signing.KeyInfo) {
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
	cmd := exec.Command(bin, "artifacts", "verify", dir, "--keys", keys, "--repo", p.repo, "--json")
	cmd.Dir = p.repo
	cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+t.TempDir(), "NSELF_CI_REPO="+p.repo)
	out, err := cmd.CombinedOutput()
	var env struct {
		Data struct {
			Verified bool `json:"verified"`
		} `json:"data"`
	}
	if jerr := json.Unmarshal(out, &env); err != nil || jerr != nil || !env.Data.Verified {
		t.Fatalf("artifacts verify: %v (%v)\n%s", err, jerr, out)
	}
	t.Logf("nself-ci artifacts verify <release> --json (exit 0): %s", bytes.TrimSpace(out))
}

// planted lists the fake rig's planted values and the keystore forms.
func (p *rig) planted() []string {
	return []string{plantedPwOne, plantedAlias, plantedPwTwo, plantedKSMark, base64.StdEncoding.EncodeToString(p.keystore), string(p.keystore)}
}
