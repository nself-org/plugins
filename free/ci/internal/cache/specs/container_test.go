//go:build container

package specs

// Container-tagged tests: they need a Docker daemon and the alpine:3.19 image
// and fail (never skip) when either is missing. The entries they restore come
// from a synthetic writer (see seedSnapshot): no real attempt writes one yet.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func needDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("docker is required for the container tests: %v", err)
	}
	if out, err := exec.Command("docker", "image", "inspect", "alpine:3.19", "--format", "{{.Id}}").CombinedOutput(); err != nil {
		t.Fatalf("image alpine:3.19 is required: %v %s", err, out)
	}
}

// runInContainer runs script in alpine with the session's environment and mounts.
func runInContainer(t *testing.T, e *env, s *Session, id, script string) (int, string) {
	t.Helper()
	envVars, mounts := s.Setup()
	var ms []ciexec.Mount
	for _, m := range mounts {
		ms = append(ms, ciexec.Mount(m))
	}
	var out strings.Builder
	ex := ciexec.Executor{Isolation: ciexec.BaselineIsolator{}}
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: id, CoordinatorID: "cachetest",
		Job:     model.Job{Kind: "test", Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}, Env: envVars},
		Command: []string{"sh", "-c", script}, Home: filepath.Join(e.home, "exec"), Mounts: ms, Output: func(b []byte) { out.Write(b) }})
	if res.Err != nil {
		t.Fatalf("container run: %v\n%s", res.Err, out.String())
	}
	return res.ExitCode, out.String()
}

// TestCacheReadOnlyMount: a restored cache mounted read-only in the job
// container refuses writes (EROFS) and the host directory stays unchanged.
func TestCacheReadOnlyMount(t *testing.T) {
	needDocker(t)
	e := newEnv(t)
	e.seedSnapshot("w1", map[string]string{"mod": "owner-mod", "build": "owner-build"})
	s := e.beginUntrustedContainer("u1")
	if got := resultsOf(e.uses("u1")); strings.Join(got, ",") != "go:readonly" {
		t.Fatalf("uses %v", got)
	}
	_, mounts := s.Setup()
	if len(mounts) != 2 || !mounts[0].ReadOnly || !mounts[1].ReadOnly {
		t.Fatalf("mounts %+v", mounts)
	}
	code, out := runInContainer(t, e, s, "u1", `echo planted > "$GOMODCACHE/new.txt"; echo "exit=$?"; echo changed > "$GOMODCACHE/f.txt"; echo "exit=$?"; cat "$GOMODCACHE/f.txt"`)
	if code != 0 || strings.Count(out, "exit=1") != 2 || !strings.Contains(out, "Read-only file system") || !strings.Contains(out, "owner-mod") {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if got := tree(t, s.dir("mod")); strings.Join(got, "|") != "f.txt -rw-r--r-- owner-mod" {
		t.Fatalf("host dir changed: %v", got)
	}
}

// TestCacheRoundTrip: a cache saved after a passing attempt is restored into a
// fresh attempt and a container job reads back byte-identical content.
func TestCacheRoundTrip(t *testing.T) {
	needDocker(t)
	e := newEnv(t)
	body := strings.Repeat("round trip bytes\x00\x01\x02", 4000)
	e.seedSnapshot("w1", map[string]string{"mod": body, "build": "b"})
	s := e.beginUntrustedContainer("u1")
	sum := sha256.Sum256([]byte(body))
	code, out := runInContainer(t, e, s, "u1", `sha256sum "$GOMODCACHE/f.txt"`)
	if code != 0 || !strings.HasPrefix(strings.TrimSpace(out), hex.EncodeToString(sum[:])) {
		t.Fatalf("exit %d, output %q, want sha256 %x", code, out, sum)
	}
	if got := getFile(t, filepath.Join(s.dir("mod"), "f.txt")); got != body {
		t.Fatal("restored bytes differ from the saved bytes")
	}
}
