package fakecloud

// Purpose: a fake cloud for the burst lifecycle (Epic D13, R-DX-20): an
// in-memory burst.Provider with injectable failures, so the lifecycle, the
// sweeper and the conformance kit run without a network, a token or a bill.
// Inputs:  burst.Provider calls and the fault switches (Outage, FailProvision,
// LoseResponse, FailTeardown, SlowProvision) plus AddForeign for resources the
// lifecycle did not create (another coordinator's, unlabelled, on an excluded address).
// Outputs: resources with provider-clock CreatedAt, deterministic ids and
// documentation-range addresses (192.0.2.0/24).
// Constraints: no clock reads of its own (the Clock is injected, so a test steps
// time instead of sleeping); no goroutines; Provision is idempotent by the intent
// label; Teardown of a missing resource succeeds; every method honours a
// cancelled context before it changes anything.

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

// Clock is a steppable wall clock shared by the cloud, the store and the engine of a test.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock starts a clock at t.
func NewClock(t time.Time) *Clock { return &Clock{t: t} }

// Now reads the clock.
func (c *Clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// Errors the fake raises on request.
var (
	// ErrLostResponse: the machine was created but the caller never learned its id
	// (a dropped connection, a coordinator crash after create).
	ErrLostResponse = errors.New("fakecloud: response lost after create")
	// ErrBadIntent: the intent lacks a lifecycle label or a label is too long.
	ErrBadIntent = errors.New("fakecloud: intent labels are not valid")
)

// Cloud is the fake provider.
type Cloud struct {
	// Latency answers Estimate (0 is "unknown"); ClassName overrides the provider class (default cloud).
	Latency   int64
	ClassName string

	id    string
	clock *Clock
	mu    sync.Mutex
	res   map[string]burst.Resource
	seq   int
	f     faults
	// Calls counts the provider calls by name (Provision, Teardown, List, Estimate).
	Calls map[string]int
}

type faults struct {
	outage        bool
	provisionErr  error
	lose          int
	teardownErr   error
	slow          time.Duration
	provisionErrN int
}

// New builds an empty cloud with the id "fakecloud".
func New(clock *Clock) *Cloud {
	return &Cloud{id: "fakecloud", clock: clock, res: map[string]burst.Resource{}, Calls: map[string]int{}}
}

// SetID renames the provider (a test with two providers).
func (c *Cloud) SetID(id string) { c.id = id }

// Outage makes every call fail with burst.ErrUnavailable until it is switched off.
func (c *Cloud) Outage(on bool) { c.mu.Lock(); c.f.outage = on; c.mu.Unlock() }

// FailProvision makes the next n Provision calls return err before creating anything.
func (c *Cloud) FailProvision(err error, n int) {
	c.mu.Lock()
	c.f.provisionErr, c.f.provisionErrN = err, n
	c.mu.Unlock()
}

// LoseResponse makes the next n Provision calls create the machine and then return ErrLostResponse.
func (c *Cloud) LoseResponse(n int) { c.mu.Lock(); c.f.lose = n; c.mu.Unlock() }

// FailTeardown makes every Teardown return err (nil clears it).
func (c *Cloud) FailTeardown(err error) { c.mu.Lock(); c.f.teardownErr = err; c.mu.Unlock() }

// SlowProvision makes each Provision take d of clock time.
func (c *Cloud) SlowProvision(d time.Duration) { c.mu.Lock(); c.f.slow = d; c.mu.Unlock() }

// AddForeign puts a machine the lifecycle did not create into the cloud and returns its id.
func (c *Cloud) AddForeign(labels map[string]string, addrs ...netip.Addr) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.create("foreign", labels, addrs, c.clock.Now()).ID
}

// Age moves a machine's creation time back by d (a machine that has been running a long time).
func (c *Cloud) Age(id string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.res[id]; ok {
		r.CreatedAt = r.CreatedAt.Add(-d)
		c.res[id] = r
	}
}

// Relabel replaces a machine's labels (a machine that stopped being the lifecycle's, e.g. promoted to a deploy host).
func (c *Cloud) Relabel(id string, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.res[id]; ok {
		r.Labels = labels
		c.res[id] = r
	}
}

// All lists every machine, labelled or not, sorted by id (for assertions).
func (c *Cloud) All() []burst.Resource {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]burst.Resource, 0, len(c.res))
	for _, r := range c.res {
		out = append(out, clone(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Has reports whether the machine still exists.
func (c *Cloud) Has(id string) bool { c.mu.Lock(); defer c.mu.Unlock(); _, ok := c.res[id]; return ok }

func clone(r burst.Resource) burst.Resource {
	l := make(map[string]string, len(r.Labels))
	for k, v := range r.Labels {
		l[k] = v
	}
	r.Labels = l
	r.Addrs = append([]netip.Addr(nil), r.Addrs...)
	return r
}

func (c *Cloud) create(name string, labels map[string]string, addrs []netip.Addr, at time.Time) burst.Resource {
	c.seq++
	id := fmt.Sprintf("fc-%04d", c.seq)
	if len(addrs) == 0 {
		addrs = []netip.Addr{netip.AddrFrom4([4]byte{192, 0, 2, byte(c.seq % 250)})}
	}
	r := burst.Resource{ID: id, Name: name, Labels: map[string]string{}, Addrs: addrs, CreatedAt: at}
	for k, v := range labels {
		r.Labels[k] = v
	}
	c.res[id] = r
	return clone(r)
}

// ID implements burst.Provider.
func (c *Cloud) ID() string { return c.id }

// Class implements burst.Provider: the cloud class unless ClassName says otherwise.
func (c *Cloud) Class() string {
	if c.ClassName == "" {
		return burst.ClassCloud
	}
	return c.ClassName
}
