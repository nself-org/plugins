package main

// Purpose: the V15 `build --artifact android` entry (P7-CACHE-15): the lane is
// synthesized from the project's required jobs or taken from its own config,
// the gh upload never overwrites, and the v1.4 key stays frozen.
// Constraints: git is a prerequisite (t.Fatal without it); no network, no gh,
// no gradle: the upload test replaces the funnel.

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/release/android"
)

// androidRepo is a committed repository with a fake Android project in android/.
func androidRepo(t *testing.T, ciYAML string) string {
	t.Helper()
	repo := t.TempDir()
	files := map[string]string{"android/gradlew": "#!/bin/sh\nexit 0\n", ".nself/ci.yaml": ciYAML}
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
