package main

// Purpose: the carrier A (SSH stdio) half of the checkout: `serve --stdio` hands
// the lease's checkout chunk stream to internal/agent/checkout, stages the
// verified bundle into the job's source tree before any step runs, and sweeps
// staging left behind by an agent that was killed.
// Inputs:  the lease and chunk frames of the stdio carrier; the agent state dir.
// Outputs: JobSetup.SourceDir for the lease; swept orphan roots.
// Constraints: chunk frames of the checkout stream never reach the session
// (it ignores them); a transfer problem fails the lease, never the process, and
// is logged once per lease; the sweep touches only this agent's own state dir
// and never a directory another live process holds (checkout.InUse), so a
// sibling `serve --stdio` keeps its files.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const maxTracked = 2048 // leases remembered per session for logging and fencing

const (
	checkoutDir     = "checkout"
	stagingMinAge   = 2 * time.Minute // covers the moment between creating a root and holding it
	jobRootMinAge   = time.Hour       // same, for roots the session made before the hook held them
	stdioHookName   = "checkout-stdio"
	stdioSweepLabel = "checkout-sweep"
)

// stdioReceiver is the receiver of this process's stdio session (one per process).
var stdioReceiver atomic.Pointer[checkout.Receiver]

// heldRoots maps a job root to the release of its owner lock.
var heldRoots sync.Map

func init() {
	must(session.RegisterJobHook(session.JobHook{Name: stdioHookName, Before: stageCheckout, After: releaseRoot}))
	must(RegisterPreStart(stdioSweepLabel, sweepStaging))
}

// withCheckout wraps the stdio carrier so lease and chunk frames reach the checkout
// receiver, and so a coordinator that stops acknowledging heartbeats fences the agent. The
// fence sits closest to the stream, so it sees the checkout chunks the receiver then consumes.
func withCheckout(car session.Carrier, state string) session.Carrier {
	return wrapStdio(car, state, newFenceCarrier)
}

func wrapStdio(car session.Carrier, state string, mkFence func(session.Carrier, string) *fenceCarrier) session.Carrier {
	r := checkout.NewReceiver(filepath.Join(state, checkoutDir), nil)
	stdioReceiver.Store(r)
	return &checkoutCarrier{Carrier: mkFence(car, state), r: r, logged: map[string]bool{}}
}

type checkoutCarrier struct {
	session.Carrier
	r      *checkout.Receiver
	logged map[string]bool // leases whose transfer problem was already reported
}

func (c *checkoutCarrier) report(lease string, err error) {
	if c.logged[lease] {
		return
	}
	if len(c.logged) >= maxTracked {
		clear(c.logged)
	}
	c.logged[lease] = true
	fmt.Fprintf(os.Stderr, "checkout: lease %s: %v\n", lease, err)
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
			if err := c.r.Expect(*b); err != nil {
				c.report(b.LeaseID, err)
			}
		case *protocol.Chunk:
			if b.Stream != checkout.Stream {
				return m, nil
			}
			epoch := uint64(0)
			if m.Epoch != nil {
				epoch = *m.Epoch
			}
			if err := c.r.Feed(m.LeaseID, epoch, *b); err != nil && !errors.Is(err, checkout.ErrUnexpected) {
				c.report(m.LeaseID, err)
			}
			continue
		}
		return m, nil
	}
}

// stageCheckout is the Before hook: it marks the job root as in use, then waits for the
// verified bundle and unpacks it.
func stageCheckout(ctx context.Context, l protocol.Lease, root session.JobRoot) (session.JobSetup, error) {
	release, err := checkout.Hold(root.Dir)
	if err != nil { // an unmarked root could be swept from under a live job
		return session.JobSetup{}, fmt.Errorf("checkout: cannot mark the job root in use: %w", err)
	}
	heldRoots.Store(root.Dir, release)
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

// releaseRoot is the After hook: the job is over, the root no longer needs its mark.
func releaseRoot(_ context.Context, _ protocol.Lease, root session.JobRoot, _ session.Outcome, _ session.Uploader) error {
	if v, ok := heldRoots.LoadAndDelete(root.Dir); ok {
		v.(func())()
	}
	return nil
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
	sweepable := func(base string, prefix string) func(string) bool {
		return func(name string) bool {
			return strings.HasPrefix(name, prefix) && !checkout.InUse(filepath.Join(base, name))
		}
	}
	co, jobs := filepath.Join(state, checkoutDir), filepath.Join(state, "jobs")
	for _, h := range []*hygiene.Host{
		{Base: co, MinAge: stagingMinAge, Logf: logf, IsAttempt: sweepable(co, "co-")},
		{Base: jobs, MinAge: jobRootMinAge, Logf: logf, IsAttempt: sweepable(jobs, "")},
	} {
		if _, err := h.SweepOrphans(ctx, nil); err != nil {
			logf("sweep %s: %v", filepath.Base(h.Base), err)
		}
	}
	return nil
}
