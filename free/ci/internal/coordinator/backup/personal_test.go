package backup

// Purpose: TestPersonalHomeBackup (P7-NODE-20, scenario audit P4): a PERSONAL home ($NSELF_CI_HOME) with one SSH
//          node (pinned key in known_hosts) and one paired agent is backed up with no serve running and
//          restored into an empty home.
// Inputs:  a real SQLite store and CA in t.TempDir(); a fake known_hosts line on a documentation address.
// Outputs: pass or fail; no network.
// Constraints: running a real job on each node needs a transport, an agent and a runner: that is outside this
//          package, so the test asserts what a job needs from the home instead (registry rows, the pinned
//          SSH host key, the agent's certificate and pinned node key, the user config) and that `nodes enable`
//          is the only step between the restore and leasing; the archive holds known_hosts and no KEK.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

const fakeKnownHosts = "[203.0.113.7]:22 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE0\n"
const fakeConfig = "ci:\n  never_hosts: [203.0.113.99]\n"

func TestPersonalHomeBackup(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("ssh-1") // an SSH node: a registry row, a pinned node key, a pin in known_hosts, no certificate
	r.addNode("agent-1")
	r.pair("agent-1") // a paired agent: certificate and pinned node key
	r.seedWork()
	for name, body := range map[string]string{"known_hosts": fakeKnownHosts, "config.yaml": fakeConfig} {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	arch := r.archive() // no coordinator is running: Create never needed one

	t.Run("the archive has known_hosts, the user config and no KEK", func(t *testing.T) {
		_, es, err := Read(bytes.NewReader(arch), r.kek)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, e := range es {
			got[e.Name] = true
		}
		for _, want := range []string{"state.db", "files/known_hosts", "files/config.yaml"} {
			if !got[want] {
				t.Fatalf("entry %s missing from %v", want, got)
			}
		}
		if bytes.Contains(arch, r.kek) {
			t.Fatal("the KEK is in the archive")
		}
	})

	home := t.TempDir()
	res := restoreInto(t, r, home, arch)
	st2, ca2 := r.reopen(home)

	t.Run("ssh node and paired agent survive without re-adding or re-pairing", func(t *testing.T) {
		for _, id := range []string{"ssh-1", "agent-1"} {
			n, err := st2.NodeGet(ctx, id)
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			if n.State != "needs-reconfirm" {
				t.Fatalf("%s state %q: an empty home restores every node as needs-reconfirm", id, n.State)
			}
			want, _, _, _ := r.st.PKINodeKeyGet(ctx, id)
			if got, _, ok, err := st2.PKINodeKeyGet(ctx, id); err != nil || !ok || got != want {
				t.Fatalf("%s pinned key: %q want %q (%v)", id, got, want, err)
			}
		}
		if certs, err := st2.PKICertsOfNode(ctx, "agent-1"); err != nil || len(certs) != 1 {
			t.Fatalf("agent certificate: %d %v", len(certs), err)
		}
		if !bytes.Equal(ca2.RootDER(), r.ca.RootDER()) {
			t.Fatal("another root: the agent would have to re-pair")
		}
	})
	t.Run("known_hosts and the user config are back, byte for byte", func(t *testing.T) {
		for name, body := range map[string]string{"known_hosts": fakeKnownHosts, "config.yaml": fakeConfig} {
			if got, err := os.ReadFile(filepath.Join(home, name)); err != nil || string(got) != body {
				t.Fatalf("%s: %q %v", name, got, err)
			}
		}
		if _, err := os.Stat(filepath.Join(home, "pki", "kek")); err != nil {
			t.Fatalf("KEK not installed: %v", err)
		}
	})
	t.Run("nodes enable is the only step before a job", func(t *testing.T) {
		reg := registry.New(st2)
		for _, id := range []string{"ssh-1", "agent-1"} {
			n, err := reg.SetState(ctx, id, "online", "enabled by admin", "cli:test")
			if err != nil || n.Record.State != "online" || len(n.Capability.Lifecycle.AuthorizedProjects) != 1 {
				t.Fatalf("%s after enable: %+v %v", id, n.Record.State, err)
			}
		}
		if res.Incarnation != r.coord.Incarnation+1 || res.LeasesGraced != 1 || res.TokenKeysRotated != 1 {
			t.Fatalf("incarnation %d graced %d rotated %d", res.Incarnation, res.LeasesGraced, res.TokenKeysRotated)
		}
		if strings.Contains(res.Rollback, "known_hosts") {
			t.Fatalf("nothing existed in the empty home to roll back: %q", res.Rollback)
		}
	})
}
