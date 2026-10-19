package runtime

// Purpose: the transport Driver of kind "provider" (Epic D4, NODE-02 registry). An admitted hosted
// attempt arrives here as a lease; the driver routes it to the Dispatcher named by the runner's
// identity.provider: Dispatch once (the handle is kept in the Book, so a re-run polls the same run),
// Liveness at the profile's poll interval until the run finished or vanished, Collect, bind the
// evidence to the attempt, and give the Sink one result. A cancelled context is a cancelled lease:
// the run is cancelled at the provider.
// Inputs:  model.Capability of the hosted runner, protocol.Lease, transport.Sink.
// Outputs: one sink.Result, or an error the dispatcher turns into a lost attempt.
// Constraints: the revision comes from lease.checkout.revision and nowhere else (never a ref lookup);
//          this file creates no lease and no attempt (admission is dispatch.Admit's); errors are
//          providers.Error values with safe text; a vanished run returns E704 and records no result,
//          the lease engine expires the lease one profile TTL later. Transient liveness errors are
//          retried (maxLivenessFailures in a row), anything else ends the run.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const (
	// Kind is the transport kind of every hosted runner.
	Kind                = "provider"
	defaultPoll         = 20 * time.Second
	cancelTimeout       = 30 * time.Second
	maxLivenessFailures = 5
)

// AttemptFacts are the parts of a providers.Attempt a lease does not carry. P7-HOST-03 (GitHub hosted
// dispatch) installs the source that reads them from the run; the default knows only the timeout.
type AttemptFacts struct {
	N                                     int
	Project, Repo, FetchRef, Trust, Label string
	TimeoutMs                             int64
}

// FactsSource supplies AttemptFacts for a lease.
type FactsSource func(ctx context.Context, node model.Capability, l protocol.Lease) (AttemptFacts, error)

// Driver implements transport.Driver for hosted providers.
type Driver struct {
	mu    sync.RWMutex
	book  Book
	facts FactsSource
	sleep func(context.Context, time.Duration) error
	now   func() time.Time
}

var defaultDriver = &Driver{book: NewMemBook(), facts: defaultFacts, sleep: sleepCtx, now: time.Now}

func init() {
	if err := transport.Register(defaultDriver); err != nil {
		panic(err)
	}
}

// Default returns the driver registered with the transport registry.
func Default() *Driver { return defaultDriver }

// SetBook replaces the handle book (UseStore for a durable one).
func (d *Driver) SetBook(b Book) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.book = b
}

// SetFacts replaces the AttemptFacts source; nil restores the default.
func (d *Driver) SetFacts(f FactsSource) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if f == nil {
		f = defaultFacts
	}
	d.facts = f
}

// SetClock replaces the wait and the time source; the injected clock of a test uses it.
func (d *Driver) SetClock(sleep func(context.Context, time.Duration) error, now func() time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if sleep == nil {
		sleep = sleepCtx
	}
	if now == nil {
		now = time.Now
	}
	d.sleep, d.now = sleep, now
}

// Book returns the handle book in use.
func (d *Driver) Book() Book {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.book
}

// Kind is "provider".
func (*Driver) Kind() string { return Kind }

func sleepCtx(ctx context.Context, wait time.Duration) error {
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func defaultFacts(_ context.Context, _ model.Capability, l protocol.Lease) (AttemptFacts, error) {
	f := AttemptFacts{N: 1}
	if l.Job.Timeout != "" {
		if t, err := time.ParseDuration(l.Job.Timeout); err == nil && t > 0 {
			f.TimeoutMs = t.Milliseconds()
		}
	}
	return f, nil
}

func refused(code, reason string) error {
	return providers.NewError(providers.Config, code, reason, 0, time.Time{})
}

// Run drives one hosted attempt to its result. See the file comment.
func (d *Driver) Run(ctx context.Context, node model.Capability, l protocol.Lease, sink transport.Sink) error {
	d.mu.RLock()
	book, facts, sleep, now := d.book, d.facts, d.sleep, d.now
	d.mu.RUnlock()
	id := node.Identity.Provider
	p, _ := providers.Lookup(id)
	disp, ok := p.(providers.Dispatcher)
	if !ok {
		return refused("E700", "provider "+id+" is not registered for hosted dispatch")
	}
	h, found, err := book.Get(ctx, l.AttemptID)
	if err != nil {
		return err
	}
	if found && h.Provider != id {
		return refused("E708", "attempt was dispatched to another provider")
	}
	if !found {
		attempt, err := buildAttempt(ctx, node, l, facts)
		if err != nil {
			return err
		}
		if h, err = disp.Dispatch(ctx, attempt); err != nil {
			return err
		}
		if err = book.Put(ctx, l.AttemptID, h); err != nil {
			return err
		}
	}
	if err = d.await(ctx, disp, h, sleep); err != nil {
		return err
	}
	collected, err := disp.Collect(ctx, h)
	if err != nil {
		return err
	}
	evidence, err := bindEvidence(collected, h, l, now())
	if err != nil {
		return err
	}
	return sink.Result(ctx, protocol.Result{LeaseID: l.LeaseID, Epoch: l.Epoch, Evidence: evidence})
}

func buildAttempt(ctx context.Context, node model.Capability, l protocol.Lease, facts FactsSource) (providers.Attempt, error) {
	if strings.TrimSpace(l.Checkout.Revision) == "" {
		return providers.Attempt{}, refused("E708", "lease names no revision")
	}
	f, err := facts(ctx, node, l)
	if err != nil {
		return providers.Attempt{}, err
	}
	spec, err := json.Marshal(l.Job)
	if err != nil {
		return providers.Attempt{}, refused("E708", "job cannot be encoded")
	}
	sum := sha256.Sum256(spec)
	n := f.N
	if n < 1 {
		n = 1
	}
	return providers.Attempt{ID: l.AttemptID, N: n, Project: f.Project, Repo: f.Repo, Revision: l.Checkout.Revision, FetchRef: f.FetchRef,
		SpecDigest: "sha256:" + hex.EncodeToString(sum[:]), CompatMode: string(l.CompatMode), Trust: f.Trust, Label: f.Label, Spec: spec, TimeoutMs: f.TimeoutMs}, nil
}

// await polls Liveness until the run finished. A cancelled context cancels the run at the provider.
func (d *Driver) await(ctx context.Context, disp providers.Dispatcher, h providers.Handle, sleep func(context.Context, time.Duration) error) error {
	poll := disp.Profile().Poll
	if poll <= 0 {
		poll = defaultPoll
	}
	failures := 0
	for {
		state, err := disp.Liveness(ctx, h)
		switch {
		case ctx.Err() != nil:
			return d.cancel(ctx, disp, h)
		case err != nil:
			if failures++; failures > maxLivenessFailures || !retryable(err) {
				return err
			}
		case state == providers.Finished:
			return nil
		case state == providers.Vanished:
			return providers.NewError(providers.Transient, "E704", "hosted run vanished", 0, time.Time{})
		case state == providers.Queued || state == providers.Running:
			failures = 0
		default:
			return refused("E704", "provider reported an unknown run state")
		}
		if sleep(ctx, poll) != nil {
			return d.cancel(ctx, disp, h)
		}
	}
}

func retryable(err error) bool {
	switch providers.ClassOf(err) {
	case providers.Transient, providers.Quota, providers.Outage:
		return true
	}
	return false
}

// cancel asks the provider to stop the run; the lease context is already done, so it gets its own.
func (*Driver) cancel(ctx context.Context, disp providers.Dispatcher, h providers.Handle) error {
	c, stop := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
	defer stop()
	_ = disp.Cancel(c, h)
	return ctx.Err()
}

var _ transport.Driver = (*Driver)(nil)
