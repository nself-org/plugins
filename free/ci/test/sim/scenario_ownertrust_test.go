//go:build integration

package sim

// Purpose: TestScenario3OwnerTrust, the owner-trust leg of scenario 3 (P7-NODE-17, NODE acceptance 2): an owner-trust job
//          runs to completion through the bare foreground run on a node that has a container runtime. The node is the
//          hosted Linux VM itself, reached over a real sshd on loopback with a pinned host key: a Docker sshd node cannot
//          carry a runtime without --privileged (nested mounts are denied by the container's AppArmor profile even with
//          SYS_ADMIN), and the harness never runs --privileged. The agent is the real nself-ci-agent; the isolation level
//          is the floor placement derived, delivered and enforced by the agent, which runs the job in the hardened
//          container profile (docker, uid 65532, read-only root, no capabilities).
// Inputs:  INTEGRATION=1, NSELF_SIM_HOST_NODE=1 (the workflow sets it), a Docker Engine the user may use, passwordless
//          sudo to start sshd, ssh, git. Without the opt-in the test is skipped; the workflow asserts that it ran.
// Outputs: none beyond the assertions.
// Constraints: the trust rule is untouched (operator-assigned container isolation, owner trust, floor container). The
//          sshd listens on 127.0.0.1 only, with the throwaway key of this test, and is stopped at cleanup.

import (
	"context"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// startHostSSHD starts a root sshd on a free loopback port that accepts one throwaway key for the current user and
// returns the port, the client key path and the public host key ("type base64").
func startHostSSHD(t *testing.T) (port int, clientKey, hostKey string) {
	t.Helper()
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	must := func(name string, args ...string) {
		t.Helper()
		if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	must("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", filepath.Join(dir, "host"))
	must("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", filepath.Join(dir, "client"))
	pub, err := os.ReadFile(filepath.Join(dir, "client.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "authorized_keys"), pub, 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	cfg := strings.Join([]string{
		"Port " + strconv.Itoa(port), "ListenAddress 127.0.0.1", "HostKey " + filepath.Join(dir, "host"),
		"PidFile " + filepath.Join(dir, "sshd.pid"), "AuthorizedKeysFile " + filepath.Join(dir, "authorized_keys"),
		"PasswordAuthentication no", "KbdInteractiveAuthentication no", "PubkeyAuthentication yes", "UsePAM yes",
		"StrictModes no", "AllowUsers " + me.Username, "PermitRootLogin no", ""}, "\n")
	if err = os.WriteFile(filepath.Join(dir, "sshd_config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	must("sudo", "-n", "mkdir", "-p", "/run/sshd")
	must("sudo", "-n", "/usr/sbin/sshd", "-f", filepath.Join(dir, "sshd_config"), "-E", filepath.Join(dir, "sshd.log"))
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := os.ReadFile(filepath.Join(dir, "sshd.log"))
			t.Logf("sshd log:\n%s", log)
		}
		if pid, err := os.ReadFile(filepath.Join(dir, "sshd.pid")); err == nil {
			_ = exec.Command("sudo", "-n", "kill", strings.TrimSpace(string(pid))).Run()
		}
	})
	hk, err := os.ReadFile(filepath.Join(dir, "host.pub"))
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(hk))
	return port, filepath.Join(dir, "client"), f[0] + " " + f[1]
}

// installHostAgent puts the real agent where the ssh driver looks for it in the user's home and removes it at cleanup.
func installHostAgent(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".nself", "ci-agent")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	src := buildVPSAgent(t, runtime.GOARCH)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bin, "nself-ci-agent"), data, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestScenario3OwnerTrust(t *testing.T) {
	if os.Getenv("NSELF_SIM_HOST_NODE") != "1" {
		t.Skip("set NSELF_SIM_HOST_NODE=1 on a disposable Linux VM with Docker and passwordless sudo (ci-node-scenarios.yml does)")
	}
	t.Setenv("NSELF_V15", "1")
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	// The node proves a container runtime that enforces limits before the operator assigns container isolation.
	info, err := exec.Command("docker", "info", "--format", "{{.MemoryLimit}} {{.PidsLimit}} {{.CPUCfsQuota}} {{.ServerVersion}}").Output()
	if err != nil || !strings.HasPrefix(string(info), "true true true") {
		t.Fatalf("the node has no container runtime that enforces limits: %q %v", info, err)
	}
	port, clientKey, hostKey := startHostSSHD(t)
	installHostAgent(t, me.HomeDir)
	f := newRepoFixture(t)
	pins := remote.PinnedHostKeys{Path: filepath.Join(f.home, "known_hosts")}
	if err = pins.Replace("nself-ci-vps", hostKey); err != nil {
		t.Fatal(err)
	}
	registerVPSWith(t, f, &simharness.Node{Host: "127.0.0.1", Port: port, User: me.Username}, runtime.GOARCH, true)
	useRealSSH(t, f, clientKey)

	platform := "linux/" + runtime.GOARCH
	c := matrixConfig(platform)
	j := c.Jobs["unit"]
	j.Container = &model.ContainerConfig{Image: container.CleanupImage}
	j.Run = []string{"sh", "-c", `echo "owner-trust job: uid=$(id -u) dockerenv=$(test -e /.dockerenv && echo yes || echo no) caps=$(grep CapEff /proc/self/status | tr -d '\t ')"`}
	c.Jobs["unit"] = j
	ch, out := f.startConfig(context.Background(), c, []string{platform})
	res := wait(t, ch, "the run")
	lines := explainJob(t, f, res.RunID, "unit@"+platform)
	text := out.String() + dumpLogs(f.home)
	t.Logf("run output:\n%s\nci explain:\n%s", text, lines)
	for _, want := range []string{"vps", platform, "placed"} {
		if !strings.Contains(lines, want) {
			t.Errorf("explain does not mention %q", want)
		}
	}
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s: the owner-trust job did not run to completion", res.Exit, res.Summary.Result)
	}
	for _, want := range []string{"uid=65532", "dockerenv=yes", "CapEff:0000000000000000"} {
		if !strings.Contains(text, want) {
			t.Errorf("the job did not run in the hardened container: output lacks %q", want)
		}
	}
}
