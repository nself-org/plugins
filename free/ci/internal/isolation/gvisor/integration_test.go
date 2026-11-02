//go:build integration

package gvisor

// Linux evidence for P7-TRUST-05 (Constitution 10.3): real jobs run through the executor and the
// sandboxed-container profile, with runsc registered in Docker. The P7-TRUST-04 escape fixtures
// run again here and must still fail. A missing prerequisite is a failure, never a skip.

import (
	"context"
	"fmt"
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
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type wrapper interface {
	Wrap(exec.JobSpec) (exec.JobSpec, error)
}

type wrapIsolator struct{ p wrapper }

func (w wrapIsolator) Prepare(_ context.Context, s *exec.JobSpec) error {
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
		t.Fatalf("Linux required for gVisor evidence, got %s", runtime.GOOS)
	}
	p := New(Options{})
	if av := p.Probe(context.Background()); !av.Available() {
		t.Fatalf("docker with the runsc runtime registered is required: %+v", av)
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

func runJob(t *testing.T, p wrapper, level, image, id, script string, mut func(*exec.JobSpec)) (string, exec.AttemptResult) {
	t.Helper()
	out := &capture{}
	spec := exec.JobSpec{AttemptID: id, CoordinatorID: "trust05-coord", Home: t.TempDir(), Output: out.write, Command: []string{"sh", "-c", script},
		Job: model.Job{Isolation: model.Isolation(level), Container: &model.ContainerConfig{Image: image}}, CPU: 1, MemoryMB: 128, Timeout: 60 * time.Second}
	if mut != nil {
		mut(&spec)
	}
	e := &exec.Executor{Isolation: wrapIsolator{p}, Grace: 2 * time.Second}
	res := e.Run(context.Background(), spec)
	t.Cleanup(func() { _ = exec.RuntimeRemove(context.Background(), "docker", "nself-ci-"+id) })
	return out.String(), res
}

func TestIntegrationProbeStartsRunscAndReportsFacts(t *testing.T) {
	p, _ := prerequisites(t)
	av := p.Probe(context.Background())
	if av.Level != "sandboxed-container" || av.Facts["banner"] != "gvisor" {
		t.Fatalf("%+v", av)
	}
	if got := av.Facts["platform"]; got != "systrap" && got != "ptrace" {
		t.Errorf("platform %q", got)
	}
	if v := av.Facts["runsc_version"]; v == "" || v == "unknown" {
		t.Errorf("runsc version fact missing: %+v", av.Facts)
	}
}

// The job really runs in the Sentry (its dmesg is gVisor's); the same script in the plain
// container profile does not see the banner, so the check proves something.
func TestIntegrationJobRunsInTheSentryAndPlainContainerDoesNot(t *testing.T) {
	p, image := prerequisites(t)
	const script = `dmesg 2>&1 | grep -c 'Starting gVisor'; true`
	out, res := runJob(t, p, "sandboxed-container", image, "t05-banner", script, nil)
	if res.Err != nil || strings.TrimSpace(out) != "1" {
		t.Fatalf("job did not run under gVisor: %+v\n%s", res, out)
	}
	plain := container.New(container.Options{})
	out, res = runJob(t, plain, "container", image, "t05-plain", script, nil)
	if res.Err != nil || strings.TrimSpace(out) != "0" {
		t.Fatalf("negative control: the plain container shows the gVisor banner: %+v\n%s", res, out)
	}
}

func TestIntegrationNoSocketNoMountNoCapabilitiesNoNetwork(t *testing.T) {
	p, image := prerequisites(t)
	script := `grep '^CapEff' /proc/self/status
test -e /var/run/docker.sock && echo SOCKET-PRESENT || echo SOCKET-ABSENT
test -e /run/docker.sock && echo SOCKET2-PRESENT || echo SOCKET2-ABSENT
mount -t tmpfs none /mnt 2>/dev/null && echo MOUNT-OK || echo MOUNT-DENIED
touch /rootfs-write 2>/dev/null && echo ROOT-WRITABLE || echo ROOT-READONLY
touch /tmp/ok && echo TMP-WRITABLE
echo "ifaces: $(tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' ' | tr '\n' ',')"
wget -q -T 3 -O /dev/null http://1.1.1.1/ 2>/dev/null && echo NET-REACHED || echo NET-DENIED`
	out, res := runJob(t, p, "sandboxed-container", image, "t05-caps", script, nil)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("%+v\n%s", res, out)
	}
	for _, want := range []string{"CapEff:\t0000000000000000", "SOCKET-ABSENT", "SOCKET2-ABSENT", "MOUNT-DENIED", "ROOT-READONLY", "TMP-WRITABLE", "ifaces: lo,", "NET-DENIED"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// The job never gets the agent's UID; the agent home, where the key lives, is not mounted.
func TestIntegrationJobUIDIsNotAgentAndAgentHomeUnreachable(t *testing.T) {
	p, image := prerequisites(t)
	home := t.TempDir()
	key := filepath.Join(home, "age.key")
	if err := os.WriteFile(key, []byte("AGE-SECRET-KEY-FIXTURE-NOT-REAL"), 0600); err != nil {
		t.Fatal(err)
	}
	slot := 3
	out, res := runJob(t, p, "sandboxed-container", image, "t05-uid", "id -u; id -g; cat "+key+" 2>&1; echo rc=$?", func(s *exec.JobSpec) { s.Home, s.Slot = home, slot })
	if res.Err != nil {
		t.Fatalf("%+v\n%s", res, out)
	}
	want, err := container.JobUID(slot, []int{os.Getuid(), os.Geteuid()})
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
	out, res := runJob(t, p, "sandboxed-container", image, "t05-env", `echo "v=$FIXTURE_ENV"`, func(s *exec.JobSpec) {
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

func TestIntegrationWorkspaceReadWriteAndCleanup(t *testing.T) {
	p, image := prerequisites(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "input.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	script := `cat input.txt && echo -n world > output.txt && mkdir -p out/deep && echo x > out/deep/f && ls -ld /repo out/deep`
	out, res := runJob(t, p, "sandboxed-container", image, "t05-ws", script, func(s *exec.JobSpec) { s.SourceDir = src })
	if res.Err != nil || res.Attempt.State != "passed" || !strings.HasPrefix(out, "hello") {
		t.Fatalf("job could not use its workspace (or cleanup failed): %+v\n%s", res, out)
	}
}

// A real process fork bomb is stopped by a limit and the host stays whole. Under gVisor the bomb ends
// one of three ways, and which one varies run to run: the host pids limit refuses the Sentry a stub
// process (pids.events max rises), the memory limit kills the sandbox (137), or the Sentry refuses the
// guest fork itself and the shell exits 2 while the host counter stays at 0 (seen on the GitHub ubuntu
// runner, D-1580: "sh: sleep: Out of memory", pids.events max=0). The test therefore proves what holds
// in all three, from readings the host takes itself. The job waits at a gate the host opens only after it
// found the container's cgroup and read its idle process count, so the proof never races --rm and never
// counts setup. Then: the cgroup carries pids.max=512, its count rose at least pressureMargin above idle and
// never past 512, the host spawned processes while that pressure held (bracketed by readings above it) and
// after, the bomb ends through a refused fork or a kill well inside its timeout, and the container is
// removed. The executor's class is checked for consistency: resource_limit only comes from host evidence,
// so an exit 2 with none stays a code failure.
func TestIntegrationForkBombStopsAtPidsLimit(t *testing.T) {
	for i := 0; i < 3; i++ { // the outcome of a fork bomb varies run to run, so one pass proves little
		forkBombOnce(t, fmt.Sprintf("t05-fork%d", i))
	}
}

func forkBombOnce(t *testing.T, id string) {
	p, image := prerequisites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	host := watchHost(ctx, "nself-ci-"+id)
	script := `until [ -e ` + gateFile + ` ]; do :; done; while :; do sleep 300 & done` // builtins only until the host opens the gate
	out, res := runJob(t, p, "sandboxed-container", image, id, script, func(s *exec.JobSpec) { s.Timeout = 20 * time.Second })
	seen := host.stop()
	took := time.Since(start) // the job and every host probe
	t.Logf("%s: exit %d reason %q in %s; host: %s; job output tail: %q", id, res.ExitCode, res.Reason, took.Round(time.Millisecond), seen, tail(out, 600))
	if res.Err != nil || res.Attempt.State != "failed" {
		t.Fatalf("fork bomb did not end as a failed job: %+v\n%q", res, tail(out, 1500))
	}
	if !seen.released {
		t.Fatalf("the host never released the bomb: %s", seen)
	}
	// `while :; do sleep 300 & done` never exits by itself: 2 is the shell's refused fork, 137 a kill by a limit.
	switch {
	case res.ExitCode == 137 && res.Reason != "resource_limit":
		t.Fatalf("a limit kill (137) was not classified: %+v", res)
	case res.ExitCode == 2 && res.Reason == "" && res.Attempt.FailureClass != "code":
		t.Fatalf("an unclassified exit 2 must stay a code failure: %+v", res)
	case res.ExitCode != 2 && res.ExitCode != 137:
		t.Fatalf("fork bomb ended with exit %d, not a refused fork (2) or a limit kill (137): %+v\n%q", res.ExitCode, res, tail(out, 1500))
	case res.Reason != "" && res.Reason != "resource_limit":
		t.Fatalf("fork bomb reason %q: %+v", res.Reason, res)
	}
	if res.Attempt.FailureClass == "timeout" {
		t.Fatalf("fork bomb was stopped by its timeout, not a limit: %+v", res)
	}
	if took > 60*time.Second {
		t.Fatalf("fork bomb and host probes took %s", took)
	}
	if seen.limit != "512" {
		t.Fatalf("container cgroup pids.max is %q, want the host limit 512: %s", seen.limit, seen)
	}
	if seen.peak < seen.idle+pressureMargin || seen.peak > 512 {
		t.Fatalf("container cgroup pids.current peaked at %d, want %d..512 (idle %d + %d): %s", seen.peak, seen.idle+pressureMargin, seen.idle, pressureMargin, seen)
	}
	if seen.overlapped < 3 || seen.spawnErr != "" {
		t.Fatalf("the host did not spawn under fork pressure: %s", seen)
	}
	assertGone(t, "nself-ci-"+id)
	if err := probe(context.Background(), "/bin/true"); err != nil {
		t.Fatalf("the host cannot spawn after the fork bomb: %v", err)
	}
}

func TestIntegrationMemoryHogIsKilledAndClassified(t *testing.T) {
	p, image := prerequisites(t)
	_, res := runJob(t, p, "sandboxed-container", image, "t05-mem", `tail /dev/zero`, func(s *exec.JobSpec) { s.MemoryMB = 64 })
	if res.Reason != "resource_limit" || res.ExitCode == 0 {
		t.Fatalf("memory hog not classified: %+v", res)
	}
	assertGone(t, "nself-ci-t05-mem")
}

// A sandboxed container carries the container profile's labels, so its List finds it and Teardown removes it.
func TestIntegrationLabelsListAndTeardown(t *testing.T) {
	p, image := prerequisites(t)
	listing := container.New(container.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan exec.AttemptResult, 1)
	go func() {
		e := &exec.Executor{Isolation: wrapIsolator{p}, Grace: 500 * time.Millisecond}
		done <- e.Run(ctx, exec.JobSpec{AttemptID: "t05-label", CoordinatorID: "trust05-coord", Home: t.TempDir(), CPU: 1, MemoryMB: 64,
			Command: []string{"sleep", "60"}, Job: model.Job{Isolation: "sandboxed-container", Container: &model.ContainerConfig{Image: image}}})
	}()
	found := false
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && !found {
		names, _ := listing.List(context.Background(), "nself.ci.attempt=t05-label")
		found = len(names) == 1 && names[0] == "nself-ci-t05-label"
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		t.Fatal("the sandboxed container is not listed by its attempt label")
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := p.Teardown(context.Background(), isolation.Attempt{ID: "t05-label"}); err != nil {
			t.Fatalf("teardown %d: %v", i, err)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("job did not stop after cancel")
	}
	assertGone(t, "nself-ci-t05-label")
}

func assertGone(t *testing.T, name string) {
	t.Helper()
	out, err := exec.RuntimeQuery(context.Background(), "docker", "ps", "-a", "--filter", "name="+name, "--format", "{{.Names}}")
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("container %s still exists: %q %v", name, out, err)
	}
}

// A script that only runs `exit 2` is the job's own failure: the sandbox did not die, so it stays class code.
func TestIntegrationPlainExit2StaysCode(t *testing.T) {
	p, image := prerequisites(t)
	out, res := runJob(t, p, "sandboxed-container", image, "t05-exit2", `exit 2`, nil)
	t.Logf("exit 2 readings: %q", tail(out, 600)) // what the classifier read and found nothing in
	if res.ExitCode != 2 || res.Reason != "" || res.Attempt.FailureClass != "code" {
		t.Fatalf("exit 2 must stay a code failure: %+v", res)
	}
}

// A script that lives long enough for the sampler to find its cgroup and then exits 2 leaves the kernel's
// counter at 0, so the counter alone cannot turn a job's own failure into a resource_limit.
func TestIntegrationPidsEventsStaysZeroForALongLivedExit2(t *testing.T) {
	p, image := prerequisites(t)
	out, res := runJob(t, p, "sandboxed-container", image, "t05-exit2-long", `sleep 2; exit 2`, nil)
	t.Logf("long exit 2 readings: %q", tail(out, 600))
	if res.ExitCode != 2 || res.Reason != "" || res.Attempt.FailureClass != "code" {
		t.Fatalf("exit 2 must stay a code failure: %+v", res)
	}
	if !strings.Contains(out, "pids.events: max=0 after") {
		t.Fatalf("the kernel counter was not read for a container that lived 2s: %q", tail(out, 600))
	}
}
