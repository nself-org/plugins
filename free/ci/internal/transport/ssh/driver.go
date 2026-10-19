// Package ssh is the SSH push transport, carrier A (Epic D4, D8): the
// coordinator opens `ssh <flags> <options> -- <dest> <agent> serve --stdio`
// through cli sdk/go/remote and speaks runner protocol v1 over the session.
//
// Purpose: deliver one lease to a node's agent, stream its output back, and
// map every way the session can end to one of: result, cancel, lost (E656
// transport.ssh_session_lost, no resume), host key refusal (E655).
// Inputs:  a model.Capability (the node), a protocol.Lease, a transport.Sink.
// Outputs: calls on the Sink; an error that is a *Error for transport failures.
// Constraints: the only remote command is one of two constant strings (the
// agent path plus `serve --stdio`, with `--accept-secret-values` when the
// coordinator is PERSONAL and the lease has operator secrets); no node, path
// or revision string is ever interpolated into a remote command or into ssh
// argv beyond the validated destination; the host key must be pinned in
// $NSELF_CI_HOME/known_hosts, never accepted on first use; secret values
// travel only inside protocol frames in memory.
package ssh

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Kind is the transport driver name (capability identity.transport).
const Kind = "ssh"

// The two remote commands. The path is relative to the SSH user's home.
const (
	agentCommand       = ".nself/ci-agent/bin/nself-ci-agent serve --stdio"
	agentCommandSecret = ".nself/ci-agent/bin/nself-ci-agent serve --stdio --accept-secret-values"
)

// SecretSource resolves the values of a lease's declared secret refs. It is
// consulted only for a PERSONAL coordinator; values go straight into a protocol
// frame and are never stored.
type SecretSource func(ctx context.Context, l protocol.Lease) ([]protocol.SecretValue, error)

// Driver is the ssh transport.
type Driver struct {
	// Home is the CI home ($NSELF_CI_HOME); the pin file is Home/known_hosts.
	Home string
	// IdentityFile is an optional private key path offered to every node (the operator's ssh
	// config and agent supply the key when empty).
	IdentityFile string
	// RepoDir is the repository whose revision is bundled for the node; empty means the
	// working directory, which suits a PERSONAL run and not a TEAM coordinator serving several
	// projects (the per-lease repository is a P7-SCHED wiring concern).
	RepoDir string
	// Personal marks a PERSONAL coordinator (the only one that may send secret values).
	Personal bool
	// Secrets resolves secret values for a lease; nil sends none.
	Secrets SecretSource
	// Renew extends the lease on each agent heartbeat (internal/lease Engine.Renew); an error
	// fences the lease. Run refuses to start without it unless NoRenew is set.
	Renew func(ctx context.Context, leaseID string, epoch uint64) error
	// NoRenew states that nothing renews leases (tests, a coordinator whose leases do not expire).
	NoRenew bool
	// MinAgentVersion is the oldest agent the coordinator accepts.
	MinAgentVersion string
	// Heartbeat and AgentTTL are what welcome tells the agent (defaults 10 s and 25 s, Epic D6).
	Heartbeat, AgentTTL time.Duration
	// CancelGrace bounds the wait for a result after cancel (default 3 s).
	CancelGrace time.Duration
	// HelloTimeout bounds connect, login and the wait for hello (default 12 s; ssh's own
	// ConnectTimeout of 10 s normally fires first).
	HelloTimeout time.Duration
	// Git runs bundle creation; nil uses the internal/exec funnel.
	Git checkout.Runner
	// Start starts ssh; nil uses cli sdk/go/remote Start.
	Start Starter
	// SSHVersion reports the local OpenSSH version; nil runs `ssh -V` once.
	SSHVersion func(ctx context.Context) (remote.Version, error)

	// afterBundle is a test seam: it sees the bundle file after its digest was recorded.
	afterBundle func(path string)

	verMu  sync.Mutex
	ver    remote.Version
	verSet bool
}

// Default is the driver registered for kind "ssh". The composition root sets its fields
// (Renew, Secrets, Personal, MinAgentVersion) before the first delivery.
var Default = &Driver{}

func init() {
	if err := transport.Register(Default); err != nil {
		panic(err)
	}
}

// Kind implements transport.Driver.
func (d *Driver) Kind() string { return Kind }

func (d *Driver) heartbeat() time.Duration { return orDefault(d.Heartbeat, 10*time.Second) }
func (d *Driver) agentTTL() time.Duration  { return orDefault(d.AgentTTL, 25*time.Second) }
func (d *Driver) cancelGrace() time.Duration {
	return orDefault(d.CancelGrace, 3*time.Second)
}
func (d *Driver) helloTimeout() time.Duration { return orDefault(d.HelloTimeout, 12*time.Second) }

func orDefault(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

func (d *Driver) home() string {
	if d.Home != "" {
		return d.Home
	}
	if v := os.Getenv("NSELF_CI_HOME"); v != "" {
		return v
	}
	u, _ := os.UserHomeDir()
	return filepath.Join(u, ".nself", "ci")
}

func (d *Driver) start() Starter {
	if d.Start != nil {
		return d.Start
	}
	return startRemote
}

// version asks ssh once; a failure is not remembered, so a transient one does not stick.
func (d *Driver) version(ctx context.Context) (remote.Version, error) {
	if d.SSHVersion != nil {
		return d.SSHVersion(ctx)
	}
	d.verMu.Lock()
	defer d.verMu.Unlock()
	if d.verSet {
		return d.ver, nil
	}
	v, err := remote.SSHVersion(ctx)
	if err != nil {
		return v, err
	}
	d.ver, d.verSet = v, true
	return v, nil
}

// command picks the constant remote command for a lease.
func (d *Driver) command(l protocol.Lease) string {
	if d.Personal && len(l.Secrets) > 0 && d.Secrets != nil {
		return agentCommandSecret
	}
	return agentCommand
}

var _ transport.Driver = (*Driver)(nil)
