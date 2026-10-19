//go:build integration

package lifecycle

// Purpose: the Docker rig of TestNodesLifecycle: three Debian sshd nodes (a jump host and two targets,
// one of them reachable only through the jump host), the real nself-ci-agent cross-built for them, an
// ssh_config the test's own ssh wrapper applies, and Deps wired to the real ssh, scp and probe.
// Inputs:  INTEGRATION=1, a Docker Engine, ssh, scp and go on the host.
// Constraints: nothing real is touched; ssh and scp run through wrappers in a temp dir that add
//          `-F <test config>` (the nodes' random ports and the fleet key cannot live in a user's own
//          ~/.ssh/config); all secrets are synthetic.

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type itRig struct {
	t      *testing.T
	fleet  *simharness.Fleet
	d      *Deps
	st     *store.Store
	dbPath string
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

// hostFingerprint is the OpenSSH SHA256 fingerprint of the node's ed25519 host key.
func (r *itRig) hostFingerprint(node string) string {
	r.t.Helper()
	f := strings.Fields(r.fleet.Exec(r.t, node, "cat", "/etc/ssh/ssh_host_ed25519_key.pub"))
	raw, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		r.t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func newITRig(t *testing.T) *itRig {
	t.Helper()
	f := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{
		{Name: "jump", Image: simharness.ImageDebian},
		{Name: "n1", Image: simharness.ImageDebian},
		{Name: "n2", Image: simharness.ImageDebian},
	}})
	r := &itRig{t: t, fleet: f, agent: buildAgent(t)}
	// The Debian image bakes its host keys, so every container starts with the same ones: give each node
	// its own, or "pinned the node's key, not the jump host's" would prove nothing.
	for _, n := range []string{"jump", "n1", "n2"} {
		f.Exec(t, n, "sh", "-c", "rm -f /etc/ssh/ssh_host_* && ssh-keygen -A >/dev/null && pkill -HUP -x sshd; sleep 1")
	}
	if a, b, c := r.hostFingerprint("jump"), r.hostFingerprint("n1"), r.hostFingerprint("n2"); a == b || b == c || a == c {
		t.Fatalf("host keys are not distinct: %s %s %s", a, b, c)
	}
	dir := t.TempDir()
	jump, n1, n2 := f.Node(t, "jump"), f.Node(t, "n1"), f.Node(t, "n2")
	cfg := "Host sim-jump\n  HostName " + jump.Host + "\n  Port " + strconv.Itoa(jump.Port) + "\n  User " + jump.User + "\n  IdentityFile " + f.KeyPath() + "\n  IdentitiesOnly yes\n" +
		"  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  LogLevel ERROR\n" +
		"Host sim-n1\n  HostName " + n1.Host + "\n  Port " + strconv.Itoa(n1.Port) + "\n  User " + n1.User + "\n  IdentityFile " + f.KeyPath() + "\n  IdentitiesOnly yes\n" +
		"Host sim-n2\n  HostName " + n2.Container + "\n  Port 22\n  User " + n2.User + "\n  IdentityFile " + f.KeyPath() + "\n  IdentitiesOnly yes\n  ProxyJump sim-jump\n"
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
	home := t.TempDir()
	r.dbPath = filepath.Join(home, "state.db")
	st, err := store.Open(r.dbPath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r.st = st
	r.d = NewDeps(st, home, "proj-it", "cli:it")
	r.d.TTY = false
	return r
}
