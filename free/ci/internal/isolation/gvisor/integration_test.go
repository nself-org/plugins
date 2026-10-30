//go:build integration

package gvisor

// Linux evidence for P7-TRUST-05 (Constitution 10.3): real jobs run through the executor and the
// sandboxed-container profile, with runsc registered in Docker. The P7-TRUST-04 escape fixtures
// run again here and must still fail. A missing prerequisite is a failure, never a skip.

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
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

func TestIntegrationForkBombStopsAtPidsLimit(t *testing.T) {
	for i := 0; i < 3; i++ { // the outcome of a fork bomb varies run to run, so one pass proves little
		forkBombOnce(t, fmt.Sprintf("t05-fork%d", i))
	}
}

func forkBombOnce(t *testing.T, id string) {
	p, image := prerequisites(t)
	start := time.Now()
	out, res := runJob(t, p, "sandboxed-container", image, id, `while :; do sleep 300 & done`, func(s *exec.JobSpec) { s.Timeout = 20 * time.Second; s.MemoryMB = 1024 })
	if res.Reason != "resource_limit" {
		t.Fatalf("fork bomb not classified: %+v\njob output tail: %q\nrunsc logs:\n%s", res, tail(out, 1500), runscLogDigest())
	}
	if took := time.Since(start); took > 60*time.Second {
		t.Fatalf("fork bomb ran %s", took)
	}
	assertGone(t, "nself-ci-"+id)
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

// runscLogDigest lists the newest runsc debug logs and their fatal-looking lines (diagnostic).
func runscLogDigest() string {
	b, _ := osexec.Command("sudo", "sh", "-c", `cd /var/log/runsc && f=$(ls -S *boot.txt | head -1); echo "== $f"; wc -l $f; tail -25 $f | cut -c1-400; echo "-- grep"; grep -n -i -E "fatal|panic|runtime:|failed to create|pids|resource temp|EAGAIN|killed|exiting|exit status" $f | head -30 | cut -c1-400`).CombinedOutput()
	return string(b)
}
