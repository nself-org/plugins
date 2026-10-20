//go:build integration

package reliability

// Purpose: the ssh carrier variant of the lab: the same Docker nodes, each with sshd and the real nself-ci-agent
//          installed for `serve --stdio`, registered as ssh nodes and reached through the real ssh client with
//          the fleet key and pinned host keys.
// Inputs:  a node count.
// Outputs: a lab whose runs travel over ssh; ssh.Default is configured the way cmd/transport_ssh.go does and
//          restored by t.Cleanup.
// Constraints: the nodes install git from the distribution (the checkout is a git bundle); images are pinned by
//          simharness; the agent is built for the node's architecture.

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// newSSHLab starts count ssh nodes.
func newSSHLab(t *testing.T, count int) *lab {
	t.Helper()
	needIntegration(t)
	bin := linuxAgent(t)
	specs := make([]simharness.NodeSpec, count)
	for i := range specs {
		specs[i] = simharness.NodeSpec{Name: "n" + itoa(i+1), Image: simharness.ImageDebian}
	}
	l := &lab{t: t, f: simharness.Start(t, simharness.Config{Nodes: specs}), state: map[string]string{}}
	l.initRepo()
	l.c = newCoord(t, t.TempDir(), nil, fast) // the store and CA only: no listener, ssh nodes dial nothing
	var wg sync.WaitGroup
	for i, s := range specs {
		l.names = append(l.names, s.Name)
		l.ids = append(l.ids, s.Name+"-"+l.f.ID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.sh(i, "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git >/dev/null")
			l.sh(i, "install -d "+agentHome+"/bin && chown -R nself:nself /home/nself/.nself")
			l.f.CopyTo(t, s.Name, bin, agentHome+"/bin/nself-ci-agent")
			l.sh(i, "chmod 755 "+agentHome+"/bin/nself-ci-agent && chown nself:nself "+agentHome+"/bin/nself-ci-agent")
		}()
	}
	wg.Wait()
	pin := remote.PinnedHostKeys{Path: filepath.Join(l.c.home, "known_hosts")}
	for i, id := range l.ids {
		f := strings.Fields(l.sh(i, "cat /etc/ssh/ssh_host_ed25519_key.pub"))
		if err := pin.Replace("nself-ci-"+id, f[0]+" "+f[1]); err != nil {
			t.Fatal(err)
		}
		n := l.f.Node(t, l.names[i])
		l.registerNode(id, func(c *model.Capability) {
			c.Identity.Provider, c.Identity.Transport = "ssh", "ssh"
			c.Identity.SSH = &model.CapabilitySSH{Hostname: n.Host, Port: n.Port, User: n.User}
		})
	}
	l.useSSH()
	return l
}

// useSSH points ssh.Default at the lab for the test.
func (l *lab) useSSH() {
	d := ssh.Default
	oldHome, oldRepo, oldID, oldMin, oldHB, oldTTL, oldGrace, oldPersonal := d.Home, d.RepoDir, d.IdentityFile, d.MinAgentVersion, d.Heartbeat, d.AgentTTL, d.CancelGrace, d.Personal
	oldRenew := d.Renew
	d.Home, d.RepoDir, d.IdentityFile, d.MinAgentVersion = l.c.home, l.root, l.f.KeyPath(), "1.5.0"
	d.Renew, d.Personal = cirun.RenewLease, true
	d.Heartbeat, d.AgentTTL, d.CancelGrace = time.Second, 6*time.Second, 3*time.Second
	l.t.Cleanup(func() {
		d.Home, d.RepoDir, d.IdentityFile, d.MinAgentVersion = oldHome, oldRepo, oldID, oldMin
		d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = oldRenew, oldPersonal, oldHB, oldTTL, oldGrace
	})
}

// killSSHSession drops node i's ssh sessions by killing the sshd session processes in the container.
func (l *lab) killSSHSession(i int) {
	l.t.Helper()
	l.sh(i, `for p in /proc/[0-9]*; do if tr '\0' ' ' < $p/cmdline 2>/dev/null | grep -q '^sshd: nself@'; then kill -9 ${p#/proc/}; fi; done; true`)
}
