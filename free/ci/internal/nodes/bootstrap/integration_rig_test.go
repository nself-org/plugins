//go:build integration

package bootstrap

// Purpose: the Docker rig of TestBootstrapDistros and TestBootstrapSlots: one simharness node of a distro
// (debian, fedora or alpine), the real nself-ci-agent cross-built for it, and a Target the real ssh and
// scp reach through wrappers that add `-F <test config>` (the node's random port and the fleet key
// cannot live in a user's own ~/.ssh/config).
// Inputs:  INTEGRATION=1, a Docker Engine, ssh, scp and go on the host.
// Constraints: nothing real is touched; the ssh user `nself` is not root, so the rig gives it
// passwordless sudo, the operator prerequisite of `nodes add --system`; all secrets are synthetic.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
)

type distro struct{ name, image, family, prep string }

var distros = []distro{
	{"debian", simharness.ImageDebian, "apt", "apt-get update -qq && apt-get install -y -qq sudo"},
	{"fedora", simharness.ImageFedora, "dnf", "dnf install -y -q sudo"},
	{"alpine", simharness.ImageAlpine, "apk", "apk add --no-cache -q sudo"},
}

type rig struct {
	t      *testing.T
	fleet  *simharness.Fleet
	target remote.Target
	agent  string
}

func nodeArch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "info", "--format", "{{.Architecture}}").Output()
	if err != nil {
		t.Fatalf("docker info: %v", err)
	}
	if a := strings.TrimSpace(string(out)); a == "aarch64" || a == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func buildAgent(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "nself-ci-agent")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", out, "./cmd/agent")
	cmd.Dir = "../../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+nodeArch(t), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, b)
	}
	return out
}

func newRig(t *testing.T, d distro) *rig {
	t.Helper()
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "n", Image: d.image}}})
	r := &rig{t: t, fleet: f, agent: buildAgent(t)}
	// The operator prerequisite of --system: an ssh user that is root or has passwordless sudo.
	r.root(d.prep + " && echo 'nself ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers")
	n := f.Node(t, "n")
	dir := t.TempDir()
	cfg := "Host sim-n\n  HostName " + n.Host + "\n  Port " + strconv.Itoa(n.Port) + "\n  User " + n.User + "\n  IdentityFile " + f.KeyPath() +
		"\n  IdentitiesOnly yes\n  UserKnownHostsFile /dev/null\n  LogLevel ERROR\n"
	cfgPath := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"ssh", "scp"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\nexec " + real + " -F " + cfgPath + " \"$@\"\n"
		if err = os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r.target = remote.Target{Dest: "sim-n", KeyPath: f.KeyPath()}
	return r
}

// run is the RunFunc over the real ssh.
func (r *rig) run(ctx context.Context, argv ...string) (string, error) {
	return remote.RunArgv(ctx, r.target, argv...)
}

func (r *rig) options(slots int) Options {
	return Options{Target: r.target, Slots: slots, Run: r.run, AgentBinary: r.agent, Logf: r.t.Logf,
		Copy: func(ctx context.Context, local, remotePath string) error {
			return remote.CopyTo(ctx, r.target, local, remotePath)
		}}
}

// root runs a script as root in the container (docker exec) and returns its output; a failing script fails the test.
func (r *rig) root(script string) string {
	r.t.Helper()
	return strings.TrimSpace(r.fleet.Exec(r.t, "n", "sh", "-c", script))
}

// try runs a script as root and returns its combined output plus the exit status, never failing the test.
func (r *rig) try(script string) (string, string) {
	r.t.Helper()
	out := r.root("{ " + script + "; } 2>&1; echo \"rc=$?\"")
	i := strings.LastIndex(out, "rc=")
	return strings.TrimSpace(out[:i]), strings.TrimSpace(out[i+3:])
}
