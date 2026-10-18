//go:build integration && linux

package hygiene

// Integration tests (Constitution §10.3: proven on a Linux host). Prerequisites are required, never
// skipped: a missing docker, passwordless sudo or useradd fails the test.

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func run(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	out, err := osexec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := run(t, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
	return out
}

func testImage() string {
	if v := os.Getenv("TRUST04_TEST_IMAGE"); v != "" {
		return v
	}
	return "alpine:3.19"
}

func TestIntegrationCleanupRemovesLabelledContainersNetworksVolumes(t *testing.T) {
	mustRun(t, "docker", "info")
	attempt := fmt.Sprintf("hy-int-%d", time.Now().UnixNano()%1_000_000)
	other := attempt + "-other"
	img := testImage()
	mustRun(t, "docker", "pull", img)
	label := "nself.ci.attempt=" + attempt
	mustRun(t, "docker", "run", "-d", "--name", "nself-ci-"+attempt, "--label", label, img, "sleep", "300")
	mustRun(t, "docker", "network", "create", "--label", label, "nself-ci-net-"+attempt)
	mustRun(t, "docker", "volume", "create", "--label", label, "nself-ci-vol-"+attempt)
	mustRun(t, "docker", "run", "-d", "--name", "nself-ci-"+other, "--label", "nself.ci.attempt="+other, img, "sleep", "300")
	mustRun(t, "docker", "run", "-d", "--name", "hy-int-unlabelled-"+attempt, img, "sleep", "300")
	t.Cleanup(func() {
		for _, n := range []string{"nself-ci-" + attempt, "nself-ci-" + other, "hy-int-unlabelled-" + attempt} {
			_, _ = run(t, "docker", "rm", "-f", n)
		}
		_, _ = run(t, "docker", "network", "rm", "nself-ci-net-"+attempt)
		_, _ = run(t, "docker", "volume", "rm", "-f", "nself-ci-vol-"+attempt)
	})
	h := &Host{Base: t.TempDir() + "/work", Runtime: &ExecRuntime{Binaries: []string{"docker"}}}
	root, err := h.JobRoot(attempt)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := h.LeakCheck(context.Background(), attempt, 0)
	if codeOf(err) != CodeLeak || !hasKind(rep, LeakContainer) || !hasKind(rep, LeakNetwork) || !hasKind(rep, LeakVolume) {
		t.Fatalf("leak check before cleanup: %v %+v", err, rep)
	}
	if err := h.Cleanup(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if rep, err := h.LeakCheck(context.Background(), attempt, 0); err != nil || !rep.Clean() {
		t.Fatalf("leak check after cleanup: %v %+v", err, rep)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("root survived")
	}
	for _, name := range []string{"nself-ci-" + other, "hy-int-unlabelled-" + attempt} {
		if out := mustRun(t, "docker", "inspect", "-f", "{{.State.Running}}", name); out != "true" {
			t.Fatalf("%s was disturbed: %s", name, out)
		}
	}
	// the sweep treats the other attempt as an orphan only when no lease owns it
	rep2, err := h.SweepOrphans(context.Background(), []string{other})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(rep2.Removed) != 0 {
		t.Fatalf("sweep removed a live attempt: %v", rep2.Removed)
	}
	if _, err := h.SweepOrphans(context.Background(), nil); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if out, _ := run(t, "docker", "inspect", "-f", "{{.State.Running}}", "nself-ci-"+other); out == "true" {
		t.Fatal("the orphan container survived the sweep")
	}
	if out := mustRun(t, "docker", "inspect", "-f", "{{.State.Running}}", "hy-int-unlabelled-"+attempt); out != "true" {
		t.Fatal("the sweep touched an unlabelled container")
	}
}

// sudoUsers creates two job users and a sudoers fixture that lets the current user run commands as
// them (the job-shim rule P7-NODE-06 installs in production), and removes both afterwards.
type sudoFixture struct {
	users    [2]string
	uids     [2]int
	rule     string
	agentUID int
}

func newSudoFixture(t *testing.T) *sudoFixture {
	t.Helper()
	mustRun(t, "sudo", "-n", "true")
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	fx := &sudoFixture{rule: "/etc/sudoers.d/nself-t09-fixture", agentUID: os.Geteuid()}
	for i := range fx.users {
		fx.users[i] = fmt.Sprintf("nsci-t09-job%d", i)
		_, _ = run(t, "sudo", "-n", "userdel", "-r", fx.users[i])
		mustRun(t, "sudo", "-n", "useradd", "-M", "-s", "/usr/sbin/nologin", fx.users[i])
		u, err := user.Lookup(fx.users[i])
		if err != nil {
			t.Fatal(err)
		}
		fx.uids[i], _ = strconv.Atoi(u.Uid)
	}
	t.Cleanup(func() {
		_, _ = run(t, "sudo", "-n", "rm", "-f", fx.rule)
		for _, u := range fx.users {
			_, _ = run(t, "sudo", "-n", "pkill", "-u", u)
			_, _ = run(t, "sudo", "-n", "userdel", "-r", u)
		}
	})
	fx.installRule(t, me.Username)
	return fx
}

func (fx *sudoFixture) installRule(t *testing.T, agent string) {
	t.Helper()
	body := fmt.Sprintf("%s ALL=(%s,%s) NOPASSWD: ALL\n", agent, fx.users[0], fx.users[1])
	tmp := filepath.Join(t.TempDir(), "rule")
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "sudo", "-n", "install", "-m", "0440", "-o", "root", "-g", "root", tmp, fx.rule)
	mustRun(t, "sudo", "-n", "visudo", "-cf", fx.rule)
}

func (fx *sudoFixture) asJob(t *testing.T, slot int, args ...string) (string, error) {
	return run(t, "sudo", append([]string{"-n", "-u", fx.users[slot], "--"}, args...)...)
}

func TestIntegrationJobShimRunsAsJobUserAndCannotReadAgentSecrets(t *testing.T) {
	fx := newSudoFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"node.key", "token", "store.db"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("agent-private fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	facts := baseFacts()
	facts.AgentUID, facts.JobShim, facts.JobUIDs, facts.Slots = fx.agentUID, true, fx.uids[:], 2
	plan, err := Separation(facts, 1)
	if err != nil || plan.Mode != JobShim || plan.UID != fx.uids[1] {
		t.Fatalf("separation with the rule: %+v %v", plan, err)
	}
	if out, err := fx.asJob(t, 1, "id", "-u"); err != nil || out != strconv.Itoa(plan.UID) {
		t.Fatalf("the shim must run the job as uid %d, got %q %v", plan.UID, out, err)
	}
	for _, f := range []string{"node.key", "token", "store.db"} {
		if out, err := fx.asJob(t, 1, "cat", filepath.Join(dir, f)); err == nil {
			t.Fatalf("the job user read %s: %q", f, out)
		}
	}
	// without the rule the policy refuses a TEAM lease with E682, and the shim really is gone
	mustRun(t, "sudo", "-n", "rm", "-f", fx.rule)
	if _, err := fx.asJob(t, 1, "id", "-u"); err == nil {
		t.Fatal("job user still reachable after the rule was removed")
	}
	facts.JobShim, facts.JobUIDs = false, nil
	facts.Coordinator = trust.Team
	if _, err := Separation(facts, 1); codeOf(err) != CodeNoJobUID {
		t.Fatalf("want E682 without the rule, got %v", err)
	}
}

func TestIntegrationConcurrentAttemptsCannotReadEachOther(t *testing.T) {
	fx := newSudoFixture(t)
	base := t.TempDir()
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Host{Base: filepath.Join(base, "work")}
	if err := os.Mkdir(h.Base, 0o711); err != nil {
		t.Fatal(err)
	}
	roots := [2]string{}
	for slot, name := range []string{"projectA-att1", "projectB-att1"} {
		root, err := h.JobRoot(name)
		if err != nil {
			t.Fatal(err)
		}
		mustRun(t, "sudo", "-n", "chown", fx.users[slot], root)
		secret := filepath.Join(root, "secret")
		mustRun(t, "sudo", "-n", "-u", fx.users[slot], "--", "sh", "-c", "echo mine > "+secret+" && chmod 600 "+secret)
		roots[slot] = root
	}
	if out, err := fx.asJob(t, 0, "cat", filepath.Join(roots[1], "secret")); err == nil {
		t.Fatalf("job A read B's workspace: %q", out)
	}
	if out, err := fx.asJob(t, 1, "cat", filepath.Join(roots[0], "secret")); err == nil {
		t.Fatalf("job B read A's workspace: %q", out)
	}
	if out, err := fx.asJob(t, 0, "cat", filepath.Join(roots[0], "secret")); err != nil || out != "mine" {
		t.Fatalf("job A cannot read its own root: %q %v", out, err)
	}
	// A runs a long process with a secret in its environment; B must not read its environ
	cmd := osexec.Command("sudo", "-n", "-u", fx.users[0], "--", "env", "HY_SECRET=fixture-only", "sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = run(t, "sudo", "-n", "pkill", "-u", fx.users[0]); _ = cmd.Wait() })
	var pid string
	for i := 0; i < 50 && pid == ""; i++ {
		pid, _ = run(t, "pgrep", "-u", strconv.Itoa(fx.uids[0]), "-x", "sleep")
		time.Sleep(100 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("job A's process never started")
	}
	pid = strings.Fields(pid)[0]
	if out, err := fx.asJob(t, 1, "cat", "/proc/"+pid+"/environ"); err == nil {
		t.Fatalf("job B read A's environ: %q", out)
	}
	if out, err := fx.asJob(t, 0, "cat", "/proc/"+pid+"/environ"); err != nil || !strings.Contains(out, "HY_SECRET") {
		t.Fatalf("job A cannot read its own environ: %q %v", out, err)
	}
	// LeakCheck by the job UID sees A's process and nobody else's, and Cleanup (as agent) kills it
	if rep, _ := (&Host{Base: h.Base}).LeakCheck(context.Background(), "projectA-att1", fx.uids[0]); !hasKind(rep, LeakProcess) {
		t.Fatalf("uid scan missed the job process: %+v", rep)
	}
	if rep, _ := (&Host{Base: h.Base}).LeakCheck(context.Background(), "projectB-att1", fx.uids[1]); hasKind(rep, LeakProcess) {
		t.Fatalf("uid scan matched another UID's process: %+v", rep)
	}
}

func TestIntegrationProbeUsernsAnswersConsistently(t *testing.T) {
	dir := t.TempDir()
	probe, err := ProbeUserns(context.Background(), dir, 200000)
	if err != nil {
		t.Fatal(err)
	}
	switch probe.Reason {
	case ProbeOK:
		if !probe.Available {
			t.Fatalf("ok must be available: %+v", probe)
		}
	case ProbeUnavail:
		if probe.Available {
			t.Fatalf("unavailable must not be available: %+v", probe)
		}
	default:
		t.Fatalf("unexpected probe result %+v (a readable probe file means the mapping does not separate)", probe)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("probe left files: %v", entries)
	}
	if _, _, err := exec.UsernsProbe(context.Background(), 0, "/etc/hostname"); err == nil {
		t.Fatal("a zero subordinate uid must be refused")
	}
}
