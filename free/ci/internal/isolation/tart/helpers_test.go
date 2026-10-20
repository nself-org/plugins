package tart

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const testImage = "ghcr.io/example/macos-base@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// rig is a profile wired to the fake tart and fake ssh in testdata. The "VM" is a directory on this machine.
type rig struct {
	t    *testing.T
	dir  string // FAKE_TART_DIR
	home string // CI home
	p    *Profile
}

func abs(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newRig(t *testing.T, mod func(*Options)) *rig {
	t.Helper()
	vmEnvPrefixes = []string{"TART_", "FAKE_TART_"}
	t.Cleanup(func() { vmEnvPrefixes = []string{"TART_"} })
	r := &rig{t: t, dir: t.TempDir(), home: t.TempDir()}
	t.Setenv("FAKE_TART_DIR", r.dir)
	for _, k := range []string{"FAKE_TART_FAIL", "FAKE_TART_NOBLOCK", "FAKE_TART_EXTRA_RUNNING", "FAKE_TART_IP", "FAKE_SSH_FAIL", "FAKE_SSH_NETCHECK", "FAKE_SSH_ENVLOG", "FAKE_SSH_OS", "FAKE_SSH_UP", "FAKE_SSH_OPEN", "FAKE_SSH_DROP", "FAKE_NETSTAT_FILE", "FAKE_NETSTAT_FAIL"} {
		t.Setenv(k, "")
	}
	opts := Options{
		Tart: abs(t, "testdata/fake-tart"), SSH: abs(t, "testdata/fake-ssh"), Netstat: abs(t, "testdata/fake-netstat"),
		BaseImage: func() string { return testImage },
		Home:      func() string { return r.home },
		GOOS:      "darwin", GOARCH: "arm64",
		Log: func(f string, a ...any) { t.Logf("tart: "+f, a...) },
	}
	if mod != nil {
		mod(&opts)
	}
	r.p = New("vm", opts)
	oldBoot, oldSSH, oldPoll, oldPause := bootTimeout, sshWait, pollEvery, destroyPause
	bootTimeout, sshWait, pollEvery, destroyPause = 20*time.Second, 3*time.Second, 100*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { bootTimeout, sshWait, pollEvery, destroyPause = oldBoot, oldSSH, oldPoll, oldPause })
	return r
}

// isolator adapts a profile to exec.Isolator the way run.selectIsolator does.
type isolator struct{ p *Profile }

func (i isolator) Prepare(ctx context.Context, s *exec.JobSpec) error {
	att := newAttempt(s)
	if err := i.p.Prepare(ctx, att); err != nil {
		return err
	}
	w, err := i.p.Wrap(*s)
	if err != nil {
		return err
	}
	*s = w
	return nil
}

func (r *rig) spec(id string, command ...string) exec.JobSpec {
	return exec.JobSpec{
		AttemptID: id, CoordinatorID: "coord-test", Home: r.home, Network: "lan",
		Job:     model.Job{Kind: "run", Isolation: "vm"},
		Command: command, Timeout: 60 * time.Second,
	}
}

func (r *rig) run(ctx context.Context, s exec.JobSpec) (exec.AttemptResult, string) {
	r.t.Helper()
	var out strings.Builder
	s.Output = func(b []byte) { out.Write(b) }
	ex := exec.Executor{Isolation: isolator{r.p}, Grace: time.Second}
	return ex.Run(ctx, s), out.String()
}

func (r *rig) read(name string) string {
	b, _ := os.ReadFile(filepath.Join(r.dir, name))
	return string(b)
}

// noClones fails the test when a clone or a marker is left.
func (r *rig) noClones() {
	r.t.Helper()
	entries, _ := os.ReadDir(filepath.Join(r.dir, "vms"))
	for _, e := range entries {
		r.t.Errorf("clone left behind: %s", e.Name())
	}
	left, _ := os.ReadDir(stateDir(r.home))
	for _, e := range left {
		r.t.Errorf("state file left behind: %s", e.Name())
	}
}

func newAttempt(s *exec.JobSpec) isolation.Attempt {
	return isolation.Attempt{ID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot}
}
