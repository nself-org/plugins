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
	// Drain stops new leases; wasOnline says whether Enable should undo it.
	Drain(ctx context.Context) (wasOnline bool, err error)
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
	Enable(ctx context.Context) error
}

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
		return res, err
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
		if err = e.step(ctx, StepDowngrade, n, "from=%s to=%s", res.From, rq.Version); err != nil {
			return res, err
		}
	}
	recs, err := n.Coordinators(ctx)
	if err != nil {
		return res, err
	}
	if cerr := CheckCeiling(recs, prep.Manifest); cerr != nil {
		return res, e.refuse(ctx, n, cerr)
	}
	wasOnline, err := n.Drain(ctx)
	if err != nil {
		return res, err
	}
	if err = e.step(ctx, StepDrain, n, "was_online=%t", wasOnline); err != nil {
		return res, e.reenable(ctx, n, wasOnline, err)
	}
	if err = e.waitIdle(ctx, n, rq); err != nil {
		return res, e.reenable(ctx, n, wasOnline, err)
	}
	if err = n.Install(ctx, InstallInput{Version: rq.Version, Staged: prep.Path, SHA256: prep.Manifest.SHA256, ManifestURL: rq.ManifestURL}); err != nil {
		// A dropped connection after the swap looks like a failed install: ask the node what it runs before deciding.
		v, verr := n.Version(context.WithoutCancel(ctx))
		switch {
		case verr != nil:
			return res, fmt.Errorf("%w (the node stays drained: its agent version is unknown: %s)", err, oneLine(verr))
		case v != res.From:
			return res, e.rollback(ctx, n, &res, wasOnline, err)
		}
		return res, e.reenable(ctx, n, wasOnline, err)
	}
	if err = e.step(ctx, StepInstall, n, "from=%s to=%s", res.From, rq.Version); err != nil {
		return res, e.rollback(ctx, n, &res, wasOnline, err)
	}
	if err = e.verifyHealth(ctx, n, rq); err != nil {
		return res, e.rollback(ctx, n, &res, wasOnline, err)
	}
	if wasOnline {
		if err = n.Enable(ctx); err != nil {
			return res, err
		}
		err = e.step(ctx, StepEnable, n, "version=%s", rq.Version)
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
			return err
		}
		if busy == 0 {
			return e.step(ctx, StepWait, n, "idle")
		}
		if !time.Now().Before(deadline) {
			return e.refuse(ctx, n, refuse(ReasonRunningJob, "node %s still runs %d job(s); nothing was installed", n.ID(), busy))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

// rollback restores the previous binary after a failure past the swap and re-checks it; the node is enabled again only when that works.
func (e *Engine) rollback(ctx context.Context, n Node, res *Result, wasOnline bool, cause error) error {
	res.RolledBack = true
	// A cancelled run must still be able to put the node right.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	if err := e.step(rctx, StepRollback, n, "from=%s reason=%s", res.To, oneLine(cause)); err != nil {
		return fmt.Errorf("E656: upgrade of %s failed (%v) and the rollback could not be recorded (%v); the node stays drained", n.ID(), oneLine(cause), oneLine(err))
	}
	var rerr error
	if rerr = n.Restore(rctx); rerr == nil {
		if rerr = n.Restart(rctx); rerr == nil {
			var got string
			if got, rerr = n.Hello(rctx, DefaultHelloWait); rerr == nil && got != res.From {
				rerr = fmt.Errorf("said hello as %s, not %s", got, res.From)
			}
		}
	}
	if rerr == nil {
		rerr = n.Probe(rctx)
	}
	if rerr != nil {
		return fmt.Errorf("E656: upgrade of %s failed (%v) and the rollback failed (%v); the node stays drained", n.ID(), oneLine(cause), oneLine(rerr))
	}
	if wasOnline {
		if err := n.Enable(rctx); err != nil {
			return fmt.Errorf("E656: upgrade of %s failed (%v); the previous agent is back but the node could not be enabled (%v)", n.ID(), oneLine(cause), oneLine(err))
		}
	}
	return fmt.Errorf("E656: upgrade of %s to %s failed and was rolled back to %s: %s", n.ID(), res.To, res.From, oneLine(cause))
}

// reenable undoes the drain after a failure before anything was installed.
func (e *Engine) reenable(ctx context.Context, n Node, wasOnline bool, cause error) error {
	if wasOnline {
		if err := n.Enable(context.WithoutCancel(ctx)); err != nil {
			return fmt.Errorf("%w (and the node could not be enabled again: %s)", cause, oneLine(err))
		}
	}
	return cause
}

// refuse audits a refusal and returns it.
func (e *Engine) refuse(ctx context.Context, n Node, rerr error) error {
	if err := e.step(ctx, StepRefused, n, "reason=%s", ReasonOf(rerr)); err != nil {
		return err
	}
	return rerr
}

func (e *Engine) step(ctx context.Context, step string, n Node, format string, a ...any) error {
	if err := e.Audit.Step(ctx, step, n.ID(), fmt.Sprintf(format, a...)); err != nil {
		return fmt.Errorf("E656: cannot record the %s step of %s: %w", step, n.ID(), err)
	}
	return nil
}

func orDur(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

func oneLine(err error) string {
	s := err.Error()
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}
