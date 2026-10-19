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
//          retried (maxLivenessFailures in a row), anything else ends the run. The handle is written
//          before anything else can fail and on a context that survives a lease cancel, so a dispatched
//          run is never unreachable; a run whose handle cannot be kept is cancelled. Every failed Run
//          forgets its handle (cancelling the run when its state is unknown), so the retry of a lost
//          attempt dispatches a fresh run; a handle of an older epoch is cancelled and replaced, one of
//          a newer epoch fences the carrier out (E708). Dormant unless compat.V15().

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// errVanished is what Run returns for a run the provider no longer knows.
var errVanished = providers.NewError(providers.Transient, "E704", "hosted run vanished", 0, time.Time{})

const (
	// Kind is the transport kind of every hosted runner.
	Kind                = "provider"
	defaultPoll         = 20 * time.Second
	cancelTimeout       = 30 * time.Second
	maxLivenessFailures = 5
)

// AttemptFacts are the parts of a providers.Attempt a lease does not carry. P7-HOST-03 (GitHub hosted
// dispatch) installs the source that reads them from the run; with none installed Run is refused.
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

// defaultFacts fails closed: a dispatcher cannot start a run without the project and repository, and
// guessing them would dispatch to the wrong repository. P7-HOST-03 installs the real source.
func defaultFacts(context.Context, model.Capability, protocol.Lease) (AttemptFacts, error) {
	return AttemptFacts{}, refused("E708", "no attempt facts source is installed")
}

func refused(code, reason string) error {
	return providers.NewError(providers.Config, code, reason, 0, time.Time{})
}

// Run drives one hosted attempt to its result. See the file comment.
func (d *Driver) Run(ctx context.Context, node model.Capability, l protocol.Lease, sink transport.Sink) (err error) {
	// compat.V15(P7-HOST-16): no hosted dispatch -> hosted dispatch through the registered provider
	if !compat.V15() {
		return refused("E708", "hosted dispatch needs v1.5")
	}
	d.mu.RLock()
	book, facts, sleep, now := d.book, d.facts, d.sleep, d.now
	d.mu.RUnlock()
	id := node.Identity.Provider
	p, _ := providers.Lookup(id)
	disp, ok := p.(providers.Dispatcher)
	if !ok {
		return refused("E700", "provider "+id+" is not registered for hosted dispatch")
	}
	h, err := attach(ctx, book, disp, id, node, l, facts)
	if err != nil {
		return err
	}
	unknown := true // the state of the run at the provider is unknown until a poll says otherwise
	keep := false   // the run finished and was collected: a failed delivery must not cost a second paid run
	defer func() {
		if err != nil && !keep {
			forget(ctx, book, disp, l.AttemptID, h, unknown && ctx.Err() == nil)
		}
	}()
	if err = d.await(ctx, disp, h, sleep); err != nil {
		unknown = !errors.Is(err, errVanished)
		return err
	}
	unknown = false
	collected, err := disp.Collect(ctx, h)
	if err != nil {
		return err
	}
	evidence, err := bindEvidence(collected, h, l, now())
	if err != nil {
		return err
	}
	keep = true
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
			cancelRun(ctx, disp, h)
			return ctx.Err()
		case err != nil:
			if failures++; failures > maxLivenessFailures || !retryable(err) {
				return err
			}
		case state == providers.Finished:
			return nil
		case state == providers.Vanished:
			return errVanished
		case state == providers.Queued || state == providers.Running:
			failures = 0
		default:
			return refused("E704", "provider reported an unknown run state")
		}
		if sleep(ctx, poll) != nil {
			cancelRun(ctx, disp, h)
			return ctx.Err()
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

var _ transport.Driver = (*Driver)(nil)
