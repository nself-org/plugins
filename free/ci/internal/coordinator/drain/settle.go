package drain

// Purpose: settle one lease at the drain deadline: a local lease is left alone; needs-operator for a job
//          that must not run twice; else cancel and wait for the lease to end or expire, then hand back.
// Inputs:  a Lease and the Drainer's Config.
// Outputs: a Result.
// Constraints: HandBack is called only after the lease deadline: an Ack means "cancel received", so after
//          it the drain waits (StopTimeout) for the lease to leave running; if the agent neither stops nor
//          lets the lease expire in time the lease is left to restart grace, never handed back early;
//          a cancelled ctx ends the wait.

import (
	"context"
	"errors"
)

const drainReason = "coordinator draining"

func (d *Drainer) settle(ctx context.Context, l Lease) Result {
	res := Result{LeaseID: l.ID}
	if l.Local { // it runs in this process: the shutdown that follows the drain ends it and requeues its pipeline
		res.Outcome = OutcomeLeftToGrace
		return res
	}
	if !l.HandBackable() {
		if _, running, err := d.cfg.Leases.Get(ctx, l.ID); err == nil && !running { // it ended on its own meanwhile
			res.Outcome = OutcomeFinished
			return res
		}
		if err := d.cfg.Leases.NeedsOperator(ctx, l, drainReason+": job must not run twice"); err != nil {
			res.Outcome, res.Err = OutcomeError, err
			return res
		}
		res.Outcome = OutcomeNeedsOperator
		return res
	}
	if d.cfg.Canceller == nil {
		res.Outcome, res.Err = OutcomeError, errors.New("drain: no canceller")
		return res
	}
	ack := d.ackChan(l.ID)
	if err := d.cfg.Canceller.Cancel(l.NodeID, l.ID, drainReason); err != nil {
		d.cfg.Logf("drain: cancel of lease %s failed: %v; waiting for its expiry", l.ID, err)
	}
	giveUp := d.cfg.Clock.Now().Add(d.cfg.AckTimeout)
	acked := false
	for {
		cur, running, err := d.cfg.Leases.Get(ctx, l.ID)
		if err != nil {
			res.Outcome, res.Err = OutcomeError, err
			return res
		}
		if !running { // the job ended (its result, cancelled or not, was committed): nothing is left to hand back
			res.Outcome = OutcomeFinished
			if acked {
				res.Outcome = OutcomeAcked
			}
			return res
		}
		now := d.cfg.Clock.Now()
		if !now.Before(cur.ExpiresAt) {
			return d.handBack(ctx, cur, true, OutcomeExpired)
		}
		if !now.Before(giveUp) {
			res.Outcome = OutcomeLeftToGrace
			return res
		}
		wait := min(d.cfg.Poll, cur.ExpiresAt.Sub(now), giveUp.Sub(now))
		select {
		case <-ctx.Done():
			res.Outcome = OutcomeAbandoned
			return res
		case <-ack:
			// the ack means "cancel received": the job and its after-job hooks may still run, so the lease is
			// handed back only once it left running or expired, never on the ack itself
			if !acked {
				acked = true
				giveUp = d.cfg.Clock.Now().Add(d.cfg.StopTimeout)
			}
			ack = nil
		case <-d.cfg.Clock.After(wait):
		}
	}
}

func (d *Drainer) handBack(ctx context.Context, l Lease, expired bool, ok Outcome) Result {
	if err := d.cfg.Leases.HandBack(ctx, l, expired, drainReason); err != nil {
		return Result{LeaseID: l.ID, Outcome: OutcomeError, Err: err}
	}
	return Result{LeaseID: l.ID, Outcome: ok}
}
