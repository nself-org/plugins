package update

// Purpose: the agent (pull) carrier's half of an upgrade (hq NODE D11): the coordinator sends the protocol `upgrade` message
//          and the agent swaps its own binary (ApplyLocal, via `nself-ci-agent upgrade`), restarts under its service manager
//          and says hello again.
// Inputs:  an AgentControl (the coordinator process's agent listener) and the lifecycle commands for the node, all injected.
// Outputs: a Node.
// Constraints: the coordinator cannot read the agent's coordinators.json, so the ceiling and the running-job check are enforced
//          by the agent (ApplyLocal) and Coordinators reports none; the coordinator side still waits for idle slots through
//          AgentControl.ActiveJobs before it sends the message; a rollback is an `upgrade` message for the version the node ran
//          before, which the agent answers by restoring its kept .prev binary (no download); the agent answers every upgrade
//          message with an upgrade-result frame (done, or refused with the E666 reason key), and Install waits for it so a
//          refusal reaches the operator as E666 with its reason; an answer that never comes (an agent that predates the frame,
//          or one that restarted before it was delivered) is not a failure, the hello that follows decides, as before.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// DefaultResultWait bounds the wait for the agent's upgrade-result (it covers the download and the verification).
const DefaultResultWait = 2 * time.Minute

// AgentControl is what the coordinator process offers for a connected pull agent.
type AgentControl interface {
	// AgentVersion is the version the node reported in its last hello.
	AgentVersion(ctx context.Context, nodeID string) (string, error)
	// ActiveJobs is the number of leases running on the node.
	ActiveJobs(ctx context.Context, nodeID string) (int, error)
	// Upgrade queues the protocol `upgrade` message on the node's control stream; allowDowngrade is its allow_downgrade field.
	Upgrade(nodeID, version, manifestURL string, allowDowngrade bool) error
	// WaitUpgradeResult waits up to wait for the agent's upgrade-result for the upgrade to version; it returns an error when
	// none arrives (ctx's error when ctx ended first).
	WaitUpgradeResult(ctx context.Context, nodeID, version string, wait time.Duration) (protocol.UpgradeResult, error)
	// WaitHello waits for a hello of the node after since and returns the agent version in it.
	WaitHello(ctx context.Context, nodeID string, since time.Time, wait time.Duration) (string, error)
}

// ErrNoAgentControl is returned when this process holds no agent listener (a plain CLI process).
var ErrNoAgentControl = errors.New("E656: this process holds no agent listener; a pull node is upgraded by the coordinator process")

var registered AgentControl

// RegisterAgentControl installs the coordinator process's AgentControl (cmd/serve registers it at start).
func RegisterAgentControl(c AgentControl) { registered = c }

// CurrentAgentControl returns the registered AgentControl, or ErrNoAgentControl.
func CurrentAgentControl() (AgentControl, error) {
	if registered == nil {
		return nil, ErrNoAgentControl
	}
	return registered, nil
}

// AgentNode upgrades an agent that connects to the coordinator.
type AgentNode struct {
	NodeID string
	Plat   lifecycle.Platform
	Ctl    AgentControl
	// DrainFn and ProbeFn are the lifecycle commands for this node.
	DrainFn func(ctx context.Context) (undo func(context.Context) error, err error)
	ProbeFn func(ctx context.Context) error
	// ResultWait bounds the wait for the agent's upgrade-result; zero means DefaultResultWait.
	ResultWait time.Duration

	from, manifestURL string
	sent              time.Time
}

var _ Node = (*AgentNode)(nil)

// ID implements Node.
func (n *AgentNode) ID() string { return n.NodeID }

// Platform implements Node.
func (n *AgentNode) Platform() lifecycle.Platform { return n.Plat }

// Version implements Node; the first answer is remembered as the version a rollback returns to.
func (n *AgentNode) Version(ctx context.Context) (string, error) {
	v, err := n.Ctl.AgentVersion(ctx, n.NodeID)
	if err == nil && n.from == "" {
		n.from = v
	}
	return v, err
}

// Coordinators is empty: the agent enforces its own ceiling (see the file purpose).
func (n *AgentNode) Coordinators(context.Context) ([]session.CoordinatorRecord, error) {
	return nil, nil
}

// Drain implements Node.
func (n *AgentNode) Drain(ctx context.Context) (func(context.Context) error, error) {
	return n.DrainFn(ctx)
}

// Busy implements Node.
func (n *AgentNode) Busy(ctx context.Context) (int, error) { return n.Ctl.ActiveJobs(ctx, n.NodeID) }

// Install sends the upgrade message (with the downgrade permission when the request has it); the agent downloads, verifies and
// swaps by itself and answers with its upgrade-result. A refusal is returned as the agent's E666 error with its reason key.
func (n *AgentNode) Install(ctx context.Context, in InstallInput) error {
	if in.ManifestURL == "" {
		return fmt.Errorf("E656: no release URL to hand to %s", n.NodeID)
	}
	n.manifestURL, n.sent = in.ManifestURL, time.Now()
	if err := n.Ctl.Upgrade(n.NodeID, in.Version, in.ManifestURL, in.AllowDowngrade); err != nil {
		return fmt.Errorf("E656: cannot send the upgrade to %s: %w", n.NodeID, err)
	}
	return n.awaitResult(ctx, in.Version)
}

// awaitResult turns the agent's upgrade-result into an error: nil for done and for no answer (the hello decides), the agent's E666
// refusal for refused, the context's error when the run was cancelled.
func (n *AgentNode) awaitResult(ctx context.Context, version string) error {
	r, err := n.Ctl.WaitUpgradeResult(ctx, n.NodeID, version, orDur(n.ResultWait, DefaultResultWait))
	if err != nil {
		return ctx.Err()
	}
	if r.Status == protocol.UpgradeRefused {
		return refuse(r.Reason, "node %s refused the upgrade to %s (%s)", n.NodeID, version, r.Reason)
	}
	return nil
}

// Restart is the agent's own: it exits after the swap and its service manager starts the new binary.
func (n *AgentNode) Restart(context.Context) error { return nil }

// Hello waits for the agent's next hello.
func (n *AgentNode) Hello(ctx context.Context, wait time.Duration) (string, error) {
	return n.Ctl.WaitHello(ctx, n.NodeID, n.sent, wait)
}

// Probe implements Node.
func (n *AgentNode) Probe(ctx context.Context) error { return n.ProbeFn(ctx) }

// Restore asks the agent to go back to the version it ran before; it restores its kept .prev binary.
func (n *AgentNode) Restore(ctx context.Context) error {
	if n.from == "" || n.manifestURL == "" {
		return fmt.Errorf("E656: %s has no recorded previous version", n.NodeID)
	}
	n.sent = time.Now()
	if err := n.Ctl.Upgrade(n.NodeID, n.from, n.manifestURL, false); err != nil {
		return fmt.Errorf("E656: cannot send the rollback to %s: %w", n.NodeID, err)
	}
	return n.awaitResult(ctx, n.from)
}
