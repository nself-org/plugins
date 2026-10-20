package update

// Purpose: the upgrade state machine of one node (hq NODE D11): verify the release, decide the direction, check the protocol
//          ceiling, drain, wait for running jobs, install with an atomic swap, restart, hello, probe, enable; any failure after the
//          swap restores the previous binary.
// Inputs:  a Node (the carrier-specific half), a Source (the verified release), an Auditor and the Request.
// Outputs: Result; an E666 *Error for a refusal, an E656 error for a failed install or health check.
// Constraints: nothing touches the node before the release verified, the direction and the ceiling passed; a node is drained
//          before anything is installed and is enabled again only when it was online before; a node with a running job is never
//          upgraded (running_job after DrainWait, the node is enabled again); a failed hello (within HelloWait) or probe restores
//          the previous binary, restarts it and re-checks it; if the restore also fails the node stays drained; every step
//          writes an audit row and a row that cannot be written stops the run.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// Defaults of a Request.
const (
	DefaultDrainWait = 10 * time.Minute
	DefaultHelloWait = 60 * time.Second
	defaultPoll      = 2 * time.Second
)

// InstallInput is what a Node installs: the verified file (carrier ssh) or the version and manifest URL the agent fetches itself.
type InstallInput struct {
	Version     string
	Staged      string
	SHA256      string // signed digest of Staged
	ManifestURL string
}

// Node is the carrier-specific half of an upgrade.
type Node interface {
	ID() string
	Platform() lifecycle.Platform
	// Version is the agent version installed now.
	Version(ctx context.Context) (string, error)
	// Coordinators are the node's recorded coordinators (<state>/coordinators.json).
	Coordinators(ctx context.Context) ([]session.CoordinatorRecord, error)
	// Drain stops new leases. undo puts the node back as it was (enabled when it was online, in maintenance when it was); nil
	// when there is nothing to put back (the admin had drained it already).
	Drain(ctx context.Context) (undo func(context.Context) error, err error)
	// Busy is the number of jobs running on the node.
	Busy(ctx context.Context) (int, error)
	// Install swaps the binary and keeps the previous one.
	Install(ctx context.Context, in InstallInput) error
	Restart(ctx context.Context) error
	// Hello waits up to wait for the agent to say hello and returns the version it reported.
	Hello(ctx context.Context, wait time.Duration) (string, error)
	Probe(ctx context.Context) error
	// Restore puts the previous binary back.
	Restore(ctx context.Context) error
}

// Downgrader is implemented by a Node that can refuse a downgrade before anything is drained (a pull agent goes back only to the binary it kept).
type Downgrader interface{ CanDowngrade() bool }

// Request is one `nodes upgrade` of one node.
type Request struct {
	Version        string
	AllowDowngrade bool
	// DrainWait bounds the wait for running jobs; HelloWait bounds the wait for the new agent's hello; Poll is the busy-check interval.
	DrainWait, HelloWait, Poll time.Duration
	// ManifestURL is handed to an agent that fetches the release itself.
	ManifestURL string
}

// Result is what an upgrade did.
type Result struct {
	Node       string    `json:"node"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Direction  Direction `json:"direction"`
	Noop       bool      `json:"noop"`
	RolledBack bool      `json:"rolled_back"`
	Note       string    `json:"note,omitempty"`
}

// Engine runs upgrades.
type Engine struct {
	Source Source
	Audit  Auditor
}

// Run upgrades one node.
func (e *Engine) Run(ctx context.Context, n Node, rq Request) (res Result, err error) {
	res = Result{Node: n.ID(), To: rq.Version}
	if res.From, err = n.Version(ctx); err != nil {
		return res, err
	}
	prep, err := e.Source.Prepare(ctx, n.Platform())
	if err != nil {
		return res, e.fail(ctx, n, err)
	}
	defer func() {
		if prep.Cleanup != nil {
			prep.Cleanup()
		}
	}()
	if err = e.step(ctx, StepVerify, n, "from=%s to=%s target=%s", res.From, rq.Version, prep.Manifest.Target); err != nil {
		return res, err
	}
	if res.Direction = DirectionOf(res.From, rq.Version); res.Direction == Same {
		res.Noop = true
		return res, e.step(ctx, StepNoop, n, "already at %s", rq.Version)
	}
	if res.Direction == Downgrade {
		if !rq.AllowDowngrade {
			return res, e.refuse(ctx, n, refuse(ReasonDowngrade, "node %s runs %s; %s is a downgrade (pass --allow-downgrade)", n.ID(), res.From, rq.Version))
		}
		if d, ok := n.(Downgrader); ok && !d.CanDowngrade() {
			return res, e.refuse(ctx, n, refuse(ReasonDowngrade, "node %s is a pull node: it goes back only to the binary it kept; run `nself-ci-agent upgrade --version %s --allow-downgrade` on the node", n.ID(), rq.Version))
		}
		if err = e.step(ctx, StepDowngrade, n, "from=%s to=%s", res.From, rq.Version); err != nil {
			return res, err
		}
	}
	recs, err := n.Coordinators(ctx)
	if err != nil {
		return res, e.fail(ctx, n, err)
	}
	if cerr := CheckCeiling(recs, prep.Manifest); cerr != nil {
		return res, e.refuse(ctx, n, cerr)
	}
	undo, err := n.Drain(ctx)
	if err != nil {
		return res, e.fail(ctx, n, err)
	}
	if err = e.step(ctx, StepDrain, n, "restore=%t", undo != nil); err != nil {
		return res, e.reenable(ctx, n, undo, err)
	}
	if err = e.waitIdle(ctx, n, rq); err != nil {
		return res, e.reenable(ctx, n, undo, err)
	}
	if err = n.Install(ctx, InstallInput{Version: rq.Version, Staged: prep.Path, SHA256: prep.Manifest.SHA256, ManifestURL: rq.ManifestURL}); err != nil {
		// A refused install (a job started during the copy) changed nothing. Any other failure may have come after the swap (a dropped
		// connection, a binary that cannot start): ask the node what it runs, and when that is unknown or new, roll back (Restore
		// puts a .prev back only when it is the version this node ran before, so it cannot go further back).
		if ReasonOf(err) == ReasonRunningJob {
			return res, e.reenable(ctx, n, undo, e.refuse(ctx, n, err))
		}
		if v, verr := n.Version(context.WithoutCancel(ctx)); verr == nil && v == res.From {
			return res, e.reenable(ctx, n, undo, e.fail(ctx, n, err))
		}
		return res, e.rollback(ctx, n, &res, undo, rq, err)
	}
	if err = e.step(ctx, StepInstall, n, "from=%s to=%s", res.From, rq.Version); err != nil {
		return res, e.rollback(ctx, n, &res, undo, rq, err)
	}
	if err = e.verifyHealth(ctx, n, rq); err != nil {
		return res, e.rollback(ctx, n, &res, undo, rq, err)
	}
	if undo != nil {
		// On a context that survives a signal: the node is upgraded and healthy here, only the put-back is left.
		uctx := context.WithoutCancel(ctx)
		if err = undo(uctx); err != nil {
			return res, e.fail(uctx, n, err)
		}
		err = e.step(uctx, StepEnable, n, "version=%s", rq.Version)
	}
	return res, err
}

// verifyHealth restarts the agent, waits for hello at the target version and re-probes.
func (e *Engine) verifyHealth(ctx context.Context, n Node, rq Request) error {
	if err := n.Restart(ctx); err != nil {
		return err
	}
	if err := e.step(ctx, StepRestart, n, "version=%s", rq.Version); err != nil {
		return err
	}
	got, err := n.Hello(ctx, orDur(rq.HelloWait, DefaultHelloWait))
	if err != nil {
		return err
	}
	if got != rq.Version {
		return fmt.Errorf("E656: node %s said hello as %s, not %s", n.ID(), got, rq.Version)
	}
	if err = e.step(ctx, StepHello, n, "version=%s", got); err != nil {
		return err
	}
	if err = n.Probe(ctx); err != nil {
		return err
	}
	return e.step(ctx, StepProbe, n, "version=%s", rq.Version)
}

// waitIdle polls until no job runs on the node, or DrainWait ends with E666 running_job.
func (e *Engine) waitIdle(ctx context.Context, n Node, rq Request) error {
	deadline := time.Now().Add(orDur(rq.DrainWait, DefaultDrainWait))
	poll := orDur(rq.Poll, defaultPoll)
	for {
		busy, err := n.Busy(ctx)
		if err != nil {
			return e.fail(ctx, n, err)
		}
		if busy == 0 {
			return e.step(ctx, StepWait, n, "idle")
		}
		if !time.Now().Before(deadline) {
			return e.refuse(ctx, n, refuse(ReasonRunningJob, "node %s still runs %d job(s); nothing was installed", n.ID(), busy))
		}
		select {
		case <-ctx.Done():
			return e.fail(context.WithoutCancel(ctx), n, ctx.Err())
		case <-time.After(poll):
		}
	}
}
