//go:build integration

package container

// Linux evidence for P7-TRUST-04 (Constitution 10.3): every test here runs a real job through
// the executor and the hardened profile. A missing prerequisite is a failure, never a skip.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type wrapIsolator struct{ p *Profile }

func (w wrapIsolator) Prepare(ctx context.Context, s *exec.JobSpec) error {
	wrapped, err := w.p.Wrap(*s)
	*s = wrapped
	return err
}

type capture struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (c *capture) write(b []byte) { c.mu.Lock(); c.buf.Write(b); c.mu.Unlock() }
func (c *capture) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.buf.String() }

func prerequisites(t *testing.T) (*Profile, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Fatalf("Linux required for container isolation evidence, got %s", runtime.GOOS)
	}
	p := New(Options{})
	if av := p.Probe(context.Background()); !av.Available() {
		t.Fatalf("container runtime required: %+v", av)
	}
	image := os.Getenv("TRUST04_TEST_IMAGE")
	if image == "" {
		image = "alpine:3.19"
	}
	if _, err := exec.RuntimeQuery(context.Background(), "docker", "inspect", image); err != nil {
		t.Fatalf("image %s must be pulled first: %v", image, err)
	}
	return p, image
}

func runJob(t *testing.T, p *Profile, image, id, script string, mut func(*exec.JobSpec)) (string, exec.AttemptResult) {
	t.Helper()
	out := &capture{}
	spec := exec.JobSpec{AttemptID: id, CoordinatorID: "trust04-coord", Home: t.TempDir(), Output: out.write, Command: []string{"sh", "-c", script},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}, CPU: 1, MemoryMB: 128, Timeout: 60 * time.Second}
	if mut != nil {
		mut(&spec)
	}
	e := &exec.Executor{Isolation: wrapIsolator{p}, Grace: 2 * time.Second}
	res := e.Run(context.Background(), spec)
	t.Cleanup(func() { _ = exec.RuntimeRemove(context.Background(), "docker", "nself-ci-"+id) })
	return out.String(), res
}

func TestIntegrationNoSocketNoMountNoCapabilities(t *testing.T) {
	p, image := prerequisites(t)
	script := `grep '^CapEff' /proc/self/status; grep '^NoNewPrivs' /proc/self/status
test -e /var/run/docker.sock && echo SOCKET-PRESENT || echo SOCKET-ABSENT
test -e /run/docker.sock && echo SOCKET2-PRESENT || echo SOCKET2-ABSENT
mount -t tmpfs none /mnt 2>/dev/null && echo MOUNT-OK || echo MOUNT-DENIED
touch /rootfs-write 2>/dev/null && echo ROOT-WRITABLE || echo ROOT-READONLY
touch /tmp/ok && echo TMP-WRITABLE
ls /sys/class/net | tr '\n' ' '; echo`
	out, res := runJob(t, p, image, "t04-caps", script, nil)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("%+v\n%s", res, out)
	}
	for _, want := range []string{"CapEff:\t0000000000000000", "NoNewPrivs:\t1", "SOCKET-ABSENT", "SOCKET2-ABSENT", "MOUNT-DENIED", "ROOT-READONLY", "TMP-WRITABLE", "lo \n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// The job never gets the agent's UID (the real, checked property); the agent home, where the
// key lives, is not mounted, so the key is unreachable by path as well. The workspace itself is
// deliberately open to the job UID, so no fixture can live there.
func TestIntegrationJobUIDIsNotAgentAndAgentHomeUnreachable(t *testing.T) {
	p, image := prerequisites(t)
	home := t.TempDir()
	key := filepath.Join(home, "age.key")
	if err := os.WriteFile(key, []byte("AGE-SECRET-KEY-FIXTURE-NOT-REAL"), 0600); err != nil {
		t.Fatal(err)
	}
	slot := 3
	out, res := runJob(t, p, image, "t04-uid", "id -u; id -g; cat "+key+" 2>&1; echo rc=$?", func(s *exec.JobSpec) { s.Home, s.Slot = home, slot })
	if res.Err != nil {
		t.Fatalf("%+v\n%s", res, out)
	}
	want, err := JobUID(slot, []int{os.Getuid(), os.Geteuid()})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || lines[0] != strconv.Itoa(want) || lines[1] != strconv.Itoa(want) {
		t.Fatalf("uid/gid %q, want %d\n%s", lines, want, out)
	}
	if want == os.Getuid() || strings.Contains(out, "AGE-SECRET-KEY") || strings.Contains(out, "rc=0") {
		t.Fatalf("agent key reachable or uid equals agent: %s", out)
	}
}

func TestIntegrationEnvArrivesThroughStdinEnvFile(t *testing.T) {
	p, image := prerequisites(t)
	home := t.TempDir()
	out, res := runJob(t, p, image, "t04-env", `echo "v=$FIXTURE_ENV"`, func(s *exec.JobSpec) {
		s.Home = home
		s.Job.Env = map[string]string{"FIXTURE_ENV": "synthetic-env-value"}
	})
	if res.Err != nil || !strings.Contains(out, "v=synthetic-env-value") {
		t.Fatalf("env did not reach the job: %+v\n%s", res, out)
	}
	if files, _ := filepath.Glob(filepath.Join(home, "ci-env-*")); len(files) != 0 {
		t.Fatalf("an env file was written to disk: %v", files)
	}
}

// The host-side UID of the job process (read from /proc on the host) is the slot UID, never the agent's.
func TestIntegrationHostUIDOfJobProcess(t *testing.T) {
	p, image := prerequisites(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan exec.AttemptResult, 1)
	slot := 2
	go func() {
		e := &exec.Executor{Isolation: wrapIsolator{p}, Grace: 500 * time.Millisecond}
		done <- e.Run(ctx, exec.JobSpec{AttemptID: "t04-hostuid", CoordinatorID: "trust04-coord", Home: t.TempDir(), CPU: 1, MemoryMB: 64, Slot: slot,
			Command: []string{"sleep", "60"}, Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}})
	}()
	want, err := JobUID(slot, []int{os.Getuid(), os.Geteuid()})
	if err != nil {
		t.Fatal(err)
	}
	var hostUID string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && hostUID == "" {
		out, err := exec.RuntimeQuery(context.Background(), "docker", "inspect", "--format", "{{.State.Pid}}", "nself-ci-t04-hostuid")
		if pid := strings.TrimSpace(string(out)); err == nil && pid != "" && pid != "0" {
			if status, rerr := os.ReadFile("/proc/" + pid + "/status"); rerr == nil {
				for _, line := range strings.Split(string(status), "\n") {
					if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "Uid:" {
						hostUID = fields[1]
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	<-done
	if hostUID == "" {
		t.Fatal("could not read the job process uid on the host")
	}
	if hostUID == strconv.Itoa(os.Getuid()) || hostUID != strconv.Itoa(want) {
		t.Fatalf("host uid of the job process is %s, want %d and not the agent's %d", hostUID, want, os.Getuid())
	}
}

func TestIntegrationWorkspaceReadWriteAndCleanup(t *testing.T) {
	p, image := prerequisites(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "input.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	script := `cat input.txt && echo -n world > output.txt && mkdir -p out/deep && echo x > out/deep/f && ls -ld /repo out/deep`
	home := t.TempDir()
	out, res := runJob(t, p, image, "t04-ws", script, func(s *exec.JobSpec) { s.SourceDir, s.Home = src, home })
	if info, err := os.Stat(filepath.Join(home, "work")); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("the workspace parent must be 0700: %v %v", info, err)
	}
	if res.Err != nil || res.Attempt.State != "passed" || !strings.HasPrefix(out, "hello") {
		t.Fatalf("job could not use its workspace (or cleanup failed): %+v\n%s", res, out)
	}
}

func TestIntegrationForkBombStopsAtPidsLimit(t *testing.T) {
	p, image := prerequisites(t)
	start := time.Now()
	_, res := runJob(t, p, image, "t04-fork", `while :; do sleep 300 & done`, func(s *exec.JobSpec) { s.Timeout = 20 * time.Second })
	if res.Reason != "resource_limit" {
		t.Fatalf("fork bomb not classified: %+v", res)
	}
	if took := time.Since(start); took > 60*time.Second {
		t.Fatalf("fork bomb ran %s", took)
	}
	assertGone(t, "nself-ci-t04-fork")
}

func TestIntegrationMemoryHogIsKilledAndClassified(t *testing.T) {
	p, image := prerequisites(t)
	_, res := runJob(t, p, image, "t04-mem", `tail /dev/zero`, func(s *exec.JobSpec) { s.MemoryMB = 64 })
	if res.Reason != "resource_limit" || res.ExitCode == 0 {
		t.Fatalf("memory hog not classified: %+v", res)
	}
	assertGone(t, "nself-ci-t04-mem")
}

func TestIntegrationLabelsListAndTeardown(t *testing.T) {
	p, image := prerequisites(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan exec.AttemptResult, 1)
	go func() {
		e := &exec.Executor{Isolation: wrapIsolator{p}, Grace: 500 * time.Millisecond}
		done <- e.Run(ctx, exec.JobSpec{AttemptID: "t04-label", CoordinatorID: "trust04-coord", Home: t.TempDir(), CPU: 1, MemoryMB: 64,
			Command: []string{"sleep", "60"}, Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}})
	}()
	var names []string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		names, _ = p.List(context.Background(), "role=ci-ephemeral")
		if contains(names, "nself-ci-t04-label") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !contains(names, "nself-ci-t04-label") {
		t.Fatalf("List by role label: %v", names)
	}
	for _, label := range []string{"nself.ci.attempt=t04-label", "coordinator_id=trust04-coord"} {
		if got, _ := p.List(context.Background(), label); !contains(got, "nself-ci-t04-label") {
			t.Errorf("List(%s) = %v", label, got)
		}
	}
	if err := p.Teardown(context.Background(), isolation.Attempt{ID: "t04-label"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Teardown(context.Background(), isolation.Attempt{ID: "t04-label"}); err != nil {
		t.Fatalf("Teardown is not idempotent: %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("job did not stop after cancel")
	}
	assertGone(t, "nself-ci-t04-label")
}

func TestIntegrationUnavailableForNeedingSeparationOnRootful(t *testing.T) {
	_, _ = prerequisites(t)
	// the runner's docker daemon is rootful: a separated node must refuse it
	sep := New(Options{Separated: func() bool { return true }, Runner: onlyBinary{"docker"}})
	if av := sep.Probe(context.Background()); av.Available() || av.Reason != isolation.ReasonRootful {
		t.Fatalf("separated node accepted a rootful runtime: %+v", av)
	}
}

func TestIntegrationRootlessPodmanIsAvailableOnSeparatedNode(t *testing.T) {
	p := New(Options{Separated: func() bool { return true }, Runner: onlyBinary{"podman"}})
	av := p.Probe(context.Background())
	if !av.Available() || av.Facts["runtime"] != "podman" || av.Facts["rootless"] != "true" {
		t.Fatalf("rootless podman not usable: %+v (install podman and run as a normal user)", av)
	}
}

// onlyBinary hides the other runtime so a probe reaches one CLI, as on a node whose agent user sees only that one.
type onlyBinary struct{ name string }

func (o onlyBinary) Query(ctx context.Context, binary string, args ...string) ([]byte, error) {
	if binary != o.name {
		return nil, os.ErrNotExist
	}
	return exec.RuntimeQuery(ctx, binary, args...)
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func assertGone(t *testing.T, name string) {
	t.Helper()
	out, err := exec.RuntimeQuery(context.Background(), "docker", "ps", "-a", "--filter", "name="+name, "--format", "{{.Names}}")
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("container %s still exists: %q %v", name, out, err)
	}
}
