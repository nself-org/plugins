//go:build container

package toolchain

// Purpose: end to end, `nself ci --container --json` runs a job inside the image
// the toolchain-image row names, and refuses with E645 while the row is pending.
// Needs a Docker daemon and fails (never skips) without one. The fixture row
// pins the alpine index digest, which is pulled by digest.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const alpineDigest = "sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8"

func run(t *testing.T, dir string, env []string, name string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return out.String(), errb.String(), code
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\njobs:\n  release-file:\n    kind: test\n    run: [sh, -c, \"cat /etc/alpine-release && test ! -w /proc/sys\"]\n"
	if err := os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=a@b", "-c", "user.name=t", "commit", "-q", "-m", "init"}} {
		if out, errs, code := run(t, repo, nil, "git", a...); code != 0 {
			t.Fatalf("git %v: %s %s", a, out, errs)
		}
	}
	return repo
}

// TestContainerRunPinnedImage: with a fixture digest row the job runs in that image (Linux or a
// Docker Desktop VM); with the shipped pending row the run is refused with E645 naming --image.
func TestContainerRunPinnedImage(t *testing.T) {
	if _, _, code := run(t, ".", nil, "docker", "info"); code != 0 {
		t.Fatal("docker is required for the container tests and `docker info` failed")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "nself-ci")
	if out, errs, code := run(t, "../..", []string{"CGO_ENABLED=0"}, "go", "build", "-mod=vendor", "-o", bin, "./cmd"); code != 0 {
		t.Fatalf("go build: %s %s", out, errs)
	}
	repo := fixtureRepo(t)
	fixture := filepath.Join(tmp, "versions.yaml")
	table := "version: 1\nrows:\n  - name: toolchain-image\n    kind: image\n    purpose: fixture\n    owner: test\n    image: docker.io/library/alpine\n    status: pinned\n    digest: " + alpineDigest + "\n    upstream: {type: none, ref: ''}\n"
	if err := os.WriteFile(fixture, []byte(table), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"NSELF_V15=1", "NSELF_CI_HOME=" + filepath.Join(tmp, "home")}

	out, errs, code := run(t, repo, append(base, "NSELF_CI_TOOLCHAIN_VERSIONS="+fixture), bin, "--container", "--json", "--no-status")
	if code != 0 {
		t.Fatalf("pinned fixture run exit %d\nstdout: %s\nstderr: %s", code, out, errs)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"container"`) {
		t.Errorf("the run did not report container isolation:\n%s", out)
	}
	if _, errs, code := run(t, "", nil, "docker", "image", "inspect", "docker.io/library/alpine@"+alpineDigest); code != 0 {
		t.Errorf("the fixture image was not pulled by digest: %s", errs)
	}

	out, errs, code = run(t, repo, base, bin, "--container", "--json", "--no-status")
	if code == 0 || !strings.Contains(out+errs, "E645") || !strings.Contains(out+errs, "--image") {
		t.Fatalf("pending row: exit %d\nstdout: %s\nstderr: %s", code, out, errs)
	}
}
