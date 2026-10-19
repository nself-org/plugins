package main

// Purpose: the composition root of the `ssh` transport (carrier A, push). It
// registers the driver in the `nself ci` binary so transport.Lookup("ssh") finds
// it, and sets ssh.Default before the first delivery (P7-AUTO-28, D-1054): lease
// renewal through the run's lease engine, the PERSONAL coordinator flag, the
// oldest agent it accepts, the agent TTL derived from the lease TTL, and the
// repository a node receives a bundle of.
// Inputs:  none. Outputs: a configured ssh.Default.
// Constraints: a secret value is sent only to an operator-owned, uid-separated
// node under a PERSONAL coordinator (NSELF_CI_COORDINATOR_MODE is not "team"):
// Personal follows the mode and Secrets is the per-lease source in internal/run.
// A TEAM coordinator keeps jobs with secrets on its own machine. The repository is pointed at once per root; a
// plain `nself ci` run serves one repository.

import (
	"sync"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// minAgentVersion is the oldest nself-ci-agent a coordinator accepts (runner protocol v1).
const minAgentVersion = "1.5.0"

// sshHeartbeat is the agent heartbeat the driver asks for.
const sshHeartbeat = 10 * time.Second

func init() { configureSSH(ssh.Default) }

// configureSSH sets d for plain `nself ci`; the driver's own tests build their own drivers.
func configureSSH(d *ssh.Driver) {
	d.Renew = cirun.RenewLease
	d.Personal = cirun.NodeSecretsAllowed() // values go to a node only under a PERSONAL coordinator (D-1101)
	d.Secrets = cirun.NodeSecretSource
	d.MinAgentVersion = minAgentVersion
	d.Heartbeat = sshHeartbeat
	// the coordinator treats an agent silent for agent_ttl + 2 heartbeats as lost: that is the lease TTL
	d.AgentTTL = cirun.RemoteLeaseTTL - 2*sshHeartbeat
	var mu sync.Mutex
	cirun.RemoteRepo = func(root string) {
		mu.Lock()
		defer mu.Unlock()
		if d.RepoDir != root {
			d.RepoDir = root
		}
	}
}
