package main

// Purpose: the V15 `build --artifact android` entry (P7-CACHE-15): the lane is
// synthesized from the project's required jobs or taken from its own config,
// the gh upload never overwrites, and the v1.4 key stays frozen.
// Constraints: git is a prerequisite (t.Fatal without it); no network, no gh,
// no gradle: the upload test replaces the funnel.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sdksigning "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/release/android"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// androidRepo is a committed repository with a fake Android project in android/.
func androidRepo(t *testing.T, ciYAML string) string {
	t.Helper()
	return androidRepoWith(t, ciYAML, "#!/bin/sh\nexit 0\n")
}

func androidRepoWith(t *testing.T, ciYAML, gradlew string) string {
	t.Helper()
	repo := t.TempDir()
	files := map[string]string{"android/gradlew": gradlew, ".nself/ci.yaml": ciYAML, ".gitignore": "app/build/\napp/release.keystore\n"}
	for name, body := range files {
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=a@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Setenv("NSELF_CI_HOME", filepath.Join(t.TempDir(), "home"))
	return repo
}

const requiredUnit = "version: 1\njobs:\n  unit:\n    kind: test\n    run: [sh, -c, \"true\"]\n"

// TestBuildV15Synthesized: with no android job in the config, the pipeline
// android-release is the required jobs plus a release job named android.
func TestBuildV15Synthesized(t *testing.T) {
	repo := androidRepo(t, requiredUnit)
	t.Setenv("ANDROID_HOME", "/opt/sdk-fixture")
	t.Setenv("JAVA_HOME", "")
	lane, err := planBuildLane(context.Background(), repo, "android", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !lane.synthesized || lane.pipeline != android.PipelineName || lane.plan.Pipeline.Name != android.PipelineName {
		t.Fatalf("lane %+v pipeline %+v", lane, lane.plan.Pipeline)
	}
	names := map[string]bool{}
	for _, j := range lane.plan.Jobs {
		names[j.Name] = true
		if j.Name != android.JobName {
			continue
		}
		jb := j.Job
		if jb.Kind != "release" || jb.Workdir != "android" || jb.Isolation != "process" || jb.Timeout != "900s" || strings.Join(jb.Deps, ",") != "unit" ||
			strings.Join(jb.Secrets, ",") != strings.Join(android.SecretRefs, ",") || *jb.Idempotent || len(jb.Env) != 1 || jb.Env["ANDROID_HOME"] != "/opt/sdk-fixture" {
			t.Fatalf("android job %+v", jb)
		}
		if len(jb.Artifacts) != 2 || jb.Artifacts[0].Path != android.APKGlob || jb.Artifacts[0].Kind != "package" {
			t.Fatalf("artifacts %+v", jb.Artifacts)
		}
	}
	if !names["unit"] || !names[android.JobName] || len(lane.plan.Jobs) != 2 {
		t.Fatalf("planned %v", names)
	}
	if len(lane.digest) != 64 {
		t.Fatalf("config digest %q", lane.digest)
	}
}

// TestBuildV15ConfiguredJob: a kind: release job named android in the project's
// config is used as it is; a job of that name of another kind is refused.
func TestBuildV15ConfiguredJob(t *testing.T) {
	own := requiredUnit + "  android:\n    kind: release\n    run: [sh, -c, \"echo own\"]\n    deps: [unit]\n    idempotent: false\n"
	repo := androidRepo(t, own)
	lane, err := planBuildLane(context.Background(), repo, "android", time.Minute)
	if err != nil || lane.synthesized {
		t.Fatalf("lane %+v err %v", lane, err)
	}
	for _, j := range lane.plan.Jobs {
		if j.Name == android.JobName && j.Job.Run[len(j.Job.Run)-1] != "echo own" {
			t.Fatalf("the configured job was replaced: %+v", j.Job.Run)
		}
	}
	bad := androidRepo(t, requiredUnit+"  android:\n    kind: test\n    run: [sh, -c, \"true\"]\n")
	if _, err = planBuildLane(context.Background(), bad, "android", time.Minute); err == nil || !strings.Contains(err.Error(), "kind: release") {
		t.Fatalf("a non-release android job was accepted: %v", err)
	}
}

// ghStub answers `gh release view` with the given assets and records every argv.
type ghStub struct {
	assets string
	calls  [][]string
}

func (g *ghStub) capture(_ context.Context, argv ...string) (string, int, error) {
	g.calls = append(g.calls, argv)
	if len(argv) > 2 && argv[2] == "view" {
		return g.assets, 0, nil
	}
	return "", 0, nil
}

func buildStderr(t *testing.T, f func() int) (string, int) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	code := f()
	_ = w.Close()
	os.Stderr = old
	b, _ := io.ReadAll(r)
	return string(b), code
}

// TestBuildV15UploadNoClobber: the upload argv carries no --clobber; an asset of
// the same name on the release is an error that names it and uploads nothing.
func TestBuildV15UploadNoClobber(t *testing.T) {
	root := androidRepo(t, requiredUnit)
	apk := filepath.Join(t.TempDir(), "app-release.apk")
	o := buildOpts{upload: true, tag: "v1.0.0", owner: "example-org", repo: "example-app"}
	old := buildCapture
	t.Cleanup(func() { buildCapture = old })

	stub := &ghStub{assets: "other.apk\n"}
	buildCapture = stub.capture
	if code := uploadBuiltV15(context.Background(), o, root, []string{apk}); code != 0 {
		t.Fatalf("upload exit %d", code)
	}
	var upload []string
	for _, c := range stub.calls {
		if c[2] == "upload" {
			upload = c
		}
	}
	want := "gh release upload v1.0.0 " + apk + " --repo example-org/example-app"
	if strings.Join(upload, " ") != want {
		t.Fatalf("upload argv %q, want %q", upload, want)
	}
	for _, c := range stub.calls {
		if strings.Contains(strings.Join(c, " "), "clobber") {
			t.Fatalf("--clobber in %v", c)
		}
	}

	stub = &ghStub{assets: "other.apk\napp-release.apk\n"}
	buildCapture = stub.capture
	msg, code := buildStderr(t, func() int { return uploadBuiltV15(context.Background(), o, root, []string{apk}) })
	if code == 0 || !strings.Contains(msg, "app-release.apk") {
		t.Fatalf("existing asset: exit %d, stderr %q", code, msg)
	}
	for _, c := range stub.calls {
		if c[2] == "upload" {
			t.Fatalf("an existing asset still reached gh release upload: %v", c)
		}
	}
}

// TestBuildV15Usage: --upload needs --tag, only android builds, the v1.4 key stays registered and the V15 entry is hidden.
func TestBuildV15Usage(t *testing.T) {
	if _, ok := subcommands["build"]; !ok {
		t.Fatal("the frozen v1.4 build key is gone")
	}
	e, ok := v15Subcommands["build"]
	if !ok || !e.Hidden || !e.V15Only || e.Flags == nil {
		t.Fatalf("v1.5 entry %+v", e)
	}
	for _, name := range []string{"artifact", "upload", "tag", "owner", "repo", "v", "verbose", "timeout"} {
		if e.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s missing: the v1.4 flags must not change", name)
		}
	}
	if msg, code := buildStderr(t, func() int { return runBuildV15([]string{"--upload"}) }); code != 1 || !strings.Contains(msg, "--tag is required") {
		t.Fatalf("exit %d %q", code, msg)
	}
	if msg, code := buildStderr(t, func() int { return runBuildV15([]string{"--artifact", "ios"}) }); code != 1 || !strings.Contains(msg, "not supported") {
		t.Fatalf("exit %d %q", code, msg)
	}
}

// fakeSyftRunner answers `version` and any scan with the golden documents.
type fakeSyftRunner struct {
	t      *testing.T
	golden string // absolute: the test changes directory
}

func (fakeSyftRunner) IsolationName() string { return "process" }
func (f fakeSyftRunner) Run(_ context.Context, c sbom.Cmd) (sbom.Result, error) {
	if len(c.Argv) == 2 && c.Argv[1] == "version" {
		return sbom.Result{Output: "Application:   syft\nVersion:       " + f.pinVersion() + "\n"}, nil
	}
	name := ""
	for i, a := range c.Argv {
		if a == "--source-name" {
			name = c.Argv[i+1]
		}
	}
	doc := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(f.golden, n))
		if err != nil {
			f.t.Fatal(err)
		}
		return []byte(strings.ReplaceAll(string(b), `"name": "fx"`, `"name": "`+name+`"`))
	}
	return sbom.Result{Files: map[string][]byte{c.Collect[0]: doc("dir.cdx.json"), c.Collect[1]: doc("dir.spdx.json")}}, nil
}

func (fakeSyftRunner) pinVersion() string { return "1.54.1" }

// absentCli is the nself cli behind the job's secret Source: nothing stored (E541), so the compat source fills the values.
type absentCli struct{}

func (absentCli) Run(_ context.Context, c secrets.Call) (secrets.Output, error) {
	if c.Argv[len(c.Argv)-1] == "--help" {
		return secrets.Output{Bytes: []byte("usage\n")}, nil
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "error": map[string]string{"code": "E541"}})
	return secrets.Output{Bytes: b, ExitCode: 1}, nil
}

// keyStore is the secret store behind the release signer.
type keyStore struct{ vals map[string][]byte }

func (keyStore) Probe(context.Context) error { return nil }
func (k keyStore) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
	var out []secrets.Value
	for _, r := range refs {
		v, ok := k.vals[r.Raw]
		if !ok {
			return nil, &secrets.Error{Code: "E676", Message: "not found", Cause: "E541"}
		}
		out = append(out, secrets.Value{Ref: r, Bytes: append([]byte(nil), v...)})
	}
	return out, nil
}

type inProcessSigner struct {
	h   signing.Handler
	res keyStore
}

func (s inProcessSigner) Sign(ctx context.Context, runID string, items []signing.Item) ([]sdksigning.Envelope, error) {
	req := signing.Request{RunID: runID, Items: items}
	raw, _ := json.Marshal(req)
	_, payloads, err := signing.DecodeRequest(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.h.Handle(ctx, req, payloads)
}

func (s inProcessSigner) PublicKey(ctx context.Context) (signing.KeyInfo, error) {
	return signing.Public(ctx, s.res, signing.KeyAttempt)
}

// TestBuildV15EndToEnd runs the real command: argv in, the lane planned, the fake gradle wrapper run on the
// operator's node with the compat-source keystore, the SBOM step, the signed release published, "APK:" and
// "Release:" printed and the verified APK uploaded without --clobber.
func TestBuildV15EndToEnd(t *testing.T) {
	gradlew := "#!/bin/sh\nset -eu\ntest \"$1\" = assembleRelease\ntest -s app/release.keystore\nmkdir -p app/build/outputs/apk/release\nprintf apk-bytes > app/build/outputs/apk/release/app-release.apk\n"
	golden, err := filepath.Abs(filepath.Join("..", "internal", "sbom", "testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}
	repo := androidRepoWith(t, requiredUnit, gradlew)
	home := os.Getenv("NSELF_CI_HOME")
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_REPO", repo)
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	t.Setenv("ANDROID_KEYSTORE_BASE64", base64.StdEncoding.EncodeToString([]byte("fake-keystore-bytes")))
	t.Setenv("ANDROID_KEYSTORE_PASSWORD", "fake-store-pass-e2e")
	t.Setenv("ANDROID_KEY_ALIAS", "fake-alias-e2e")
	t.Setenv("ANDROID_KEY_PASSWORD", "fake-key-pass-e2e")
	t.Chdir(repo)
	_, keyValue, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	res := keyStore{vals: map[string][]byte{signing.KeyRef: keyValue}}
	oldPub, oldSBOM, oldCap := cirun.ReleasePublisher, newSBOMStep, buildCapture
	t.Cleanup(func() {
		cirun.ReleasePublisher, newSBOMStep, buildCapture = oldPub, oldSBOM, oldCap
		cirun.ConfigureSecrets(cirun.SecretsRuntime{Mode: secrets.ModePersonalLocal})
	})
	cirun.ConfigureSecrets(cirun.SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: absentCli{}})
	cirun.ReleasePublisher = func(ctx context.Context, r cirun.ReleasePublishRequest) (cirun.ReleaseOutcome, error) {
		blobs, berr := cas.New(records.StoreDir(home))
		if berr != nil {
			return cirun.ReleaseOutcome{}, berr
		}
		d := publish.Deps{Src: r.Store, Blobs: blobs, Ledger: r.Store, Home: home, Root: repo, Capture: report.Capture,
			Signer: inProcessSigner{h: signing.Handler{Src: r.Store, Blobs: blobs, Project: repo, Resolver: res}, res: res}}
		rel, perr := publish.Run(ctx, d, r.RunID)
		return cirun.ReleaseOutcome{State: rel.State, Dir: rel.Dir, Message: rel.Message}, perr
	}
	pin := sbom.Pin{Version: "1.54.1", Sources: []toolchain.Source{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "https://example.invalid/syft.tgz",
		SHA256: strings.Repeat("a", 64), BinarySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("#!/bin/sh\n")))}}}
	tool := sbom.ToolPath(home, pin.Version)
	if err = os.MkdirAll(filepath.Dir(tool), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newSBOMStep = func(h string, _ sbom.Pin) android.SBOMStep {
		return android.SBOMStep{Home: h, Pin: pin, NewRunner: func(string) sbom.Runner { return fakeSyftRunner{t, golden} }}
	}
	stub := &ghStub{assets: "other.apk\n"}
	buildCapture = stub.capture

	var code int
	stdout := buildStdout(t, func() {
		code = runBuildV15([]string{"--artifact", "android", "--upload", "--tag", "v1.0.0", "--owner", "example-org", "--repo", "example-app", "--timeout", "120", "android"})
	})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stdout)
	}
	var apk, release string
	for _, l := range strings.Split(stdout, "\n") {
		if v, ok := strings.CutPrefix(l, "APK: "); ok {
			apk = v
		}
		if v, ok := strings.CutPrefix(l, "Release: "); ok {
			release = v
		}
	}
	if b, rerr := os.ReadFile(apk); rerr != nil || string(b) != "apk-bytes" || !strings.HasPrefix(apk, release) || !strings.HasPrefix(release, home) {
		t.Fatalf("APK %q release %q (%v)\n%s", apk, release, rerr, stdout)
	}
	var upload string
	for _, c := range stub.calls {
		if strings.Contains(strings.Join(c, " "), "clobber") {
			t.Fatalf("--clobber in %v", c)
		}
		if c[2] == "upload" {
			upload = strings.Join(c, " ")
		}
	}
	if upload != "gh release upload v1.0.0 "+apk+" --repo example-org/example-app" {
		t.Fatalf("upload argv %q", upload)
	}
	if _, serr := os.Stat(filepath.Join(repo, "android", "app", "release.keystore")); serr == nil {
		t.Fatal("a keystore file is in the repository")
	}
}

// buildStdout runs f and returns what it printed to stdout.
func buildStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = old
	return <-done
}
