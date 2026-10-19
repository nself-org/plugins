package main

// Purpose: the carrier A (SSH stdio) half of the checkout: `serve --stdio` hands
// the lease's checkout chunk stream to internal/agent/checkout, stages the
// verified bundle into the job's source tree before any step runs, and sweeps
// staging left behind by an agent that was killed.
// Inputs:  the lease and chunk frames of the stdio carrier; the agent state dir.
// Outputs: JobSetup.SourceDir for the lease; swept orphan roots.
// Constraints: chunk frames of the checkout stream never reach the session
// (it ignores them); a transfer problem fails the lease, never the process;
// the sweep touches only this agent's own state dir, and only roots older than
// the age that rules out a sibling `serve --stdio` still working.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const (
	checkoutDir     = "checkout"
	stagingMinAge   = 15 * time.Minute // a transfer older than this has no live owner
	jobRootMinAge   = 24 * time.Hour   // longer than any lease's expiry
	stdioHookName   = "checkout-stdio"
	stdioSweepLabel = "checkout-sweep"
)

// stdioReceiver is the receiver of this process's stdio session (one per process).
var stdioReceiver atomic.Pointer[checkout.Receiver]

func init() {
	must(session.RegisterJobHook(session.JobHook{Name: stdioHookName, Before: stageCheckout}))
	must(RegisterPreStart(stdioSweepLabel, sweepStaging))
}

// withCheckout wraps the stdio carrier so lease and chunk frames reach the checkout receiver.
func withCheckout(car session.Carrier, state string) session.Carrier {
	r := checkout.NewReceiver(filepath.Join(state, checkoutDir), nil)
	stdioReceiver.Store(r)
	return &checkoutCarrier{Carrier: car, r: r}
}

type checkoutCarrier struct {
	session.Carrier
	r *checkout.Receiver
}

// Recv announces each lease that has a checkout and consumes its chunk frames.
func (c *checkoutCarrier) Recv() (protocol.Message, error) {
	for {
		m, err := c.Carrier.Recv()
		if err != nil {
			return m, err
		}
		switch b := m.Body.(type) {
		case *protocol.Lease:
			// A refused announcement is reported by the Before hook, which fails the lease.
			_ = c.r.Expect(*b)
		case *protocol.Chunk:
			if b.Stream != checkout.Stream {
				return m, nil
			}
			epoch := uint64(0)
			if m.Epoch != nil {
				epoch = *m.Epoch
			}
			if err := c.r.Feed(m.LeaseID, epoch, *b); err != nil && !errors.Is(err, checkout.ErrUnexpected) {
				fmt.Fprintln(os.Stderr, "checkout:", err)
			}
			continue
		}
		return m, nil
	}
}

// stageCheckout is the Before hook: it waits for the verified bundle and unpacks it.
func stageCheckout(ctx context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
	if l.Checkout.Size <= 0 {
		return session.JobSetup{}, nil
	}
	r := stdioReceiver.Load()
	if r == nil {
		return session.JobSetup{}, errors.New("checkout: this agent session cannot receive a checkout")
	}
	src, err := r.Stage(ctx, l, root.Dir)
	if err != nil {
		return session.JobSetup{}, err
	}
	return session.JobSetup{SourceDir: src}, nil
}

// sweepStaging removes checkout staging and job roots of agents that died. It runs
// before serve reads or writes anything; a failed sweep is logged, never fatal.
func sweepStaging(ctx context.Context, command string) error {
	if command != "serve" {
		return nil
	}
	state, err := stateDir()
	if err != nil || !filepath.IsAbs(state) {
		return nil
	}
	logf := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "hygiene: "+format+"\n", a...) }
	for _, h := range []*hygiene.Host{
		{Base: filepath.Join(state, checkoutDir), MinAge: stagingMinAge, Logf: logf,
			IsAttempt: func(n string) bool { return strings.HasPrefix(n, "co-") }},
		{Base: filepath.Join(state, "jobs"), MinAge: jobRootMinAge, Logf: logf,
			IsAttempt: func(string) bool { return true }},
	} {
		if _, err := h.SweepOrphans(ctx, nil); err != nil {
			logf("sweep %s: %v", filepath.Base(h.Base), err)
		}
	}
	return nil
}
