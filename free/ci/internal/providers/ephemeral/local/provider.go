package local

// Purpose: burst.Provider for local ephemeral capacity (Epic D13): one job per sandbox on this
// host, a gVisor sandboxed-container (container where runsc is absent) on Linux or a Tart clone
// (`vm`) on Apple silicon, cost class `local`. The provider owns the reservation of a slot, the
// labels the sweeper finds leaks by, and the capacity accounting; the adapters own the sandbox.
// Inputs:  an Intent (token, spec, labels), a Backend over the P7-TRUST adapters, the host
//          reservation (exec.Capacity + node settings).
// Outputs: Resources whose ID is the attempt id: a runner starts its job with AttemptID = Resource.ID
//          (see Attempt), the adapter names the sandbox nself-ci-<id> and labels it role=ci-ephemeral and
//          coordinator_id, and Teardown(ID) removes it. List merges reservations with the sandboxes
//          the runtime still holds, so a coordinator killed with a job running leaves a labelled leak
//          the P7-SCHED-08 sweeper finds and removes.
// Constraints: no spend, no network, no model of a second container or VM implementation; Estimate never
//          calls the runtime; Provision is idempotent by intent (the id is the token); Teardown treats
//          an unknown id as success and an unreachable runtime as retryable (E698); a slot is refused
//          (burst.ErrQuota) beyond min(runtime limit, host-reservation share, MaxSlots).

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

const (
	roleKey, roleValue = burst.LabelRole, burst.RoleEphemeral
	coordKey           = burst.LabelCoordinator
	intentKey          = burst.LabelIntent
	vmLevel            = "vm"
	// idPrefix namespaces the attempt ids of this provider: the container profile labels every job container
	// role=ci-ephemeral, so only ids that start with it (nself-ci-burst-<token>) are ever listed or removed here.
	idPrefix = "burst-"
	// ProviderID is the id this provider reports.
	ProviderID = "ephemeral-local"
)

var (
	// tokenRe is what an intent token must look like: it becomes part of a container or VM name.
	tokenRe = regexp.MustCompile(`^[A-Za-z0-9]{1,63}$`)
	// idRe is every id this provider can have issued: burst-<token>, or the cleanup helper of one, burst-<token>-clean.
	idRe = regexp.MustCompile(`^burst-[A-Za-z0-9]{1,63}(-clean)?$`)
)

// Options configures a Provider; every field but Backend has a default.
type Options struct {
	// Backend is the platform's view of the adapters (NewContainerBackend or NewTartBackend).
	Backend Backend
	// Host reads the host capacity and the node's stored reservation. Default: exec.Probe and no reservation.
	Host func() (exec.Capacity, reservation.Settings)
	// MaxSlots caps concurrent sandboxes below what the host would allow (0 = no extra cap).
	MaxSlots int
	// Lanes reads how many regular (non-burst) lane jobs run on this node now; they hold slots burst must not
	// grant again. Default: none known (the dispatch wiring of the coordinator supplies it).
	Lanes func() int
	Now   func() time.Time
}

// Provider is the local ephemeral provider.
type Provider struct {
	opts Options
	lat  *latencies

	provSem chan struct{} // serialises Provision (capacity check and reservation are one step); a waiter gives up with its ctx
	mu      sync.Mutex
	ledger  map[string]entry
	level   model.Isolation // last level a probe offered; Estimate reads it without calling the runtime
}

type entry struct {
	res   burst.Resource
	level model.Isolation
}

var _ burst.Provider = (*Provider)(nil)

// New returns a Provider over opts.Backend.
func New(o Options) (*Provider, error) {
	if o.Backend == nil {
		return nil, unavailable(ReasonNoAdapter, nil, "no backend")
	}
	if o.Host == nil {
		o.Host = func() (exec.Capacity, reservation.Settings) { return exec.Probe(), reservation.Settings{} }
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Provider{opts: o, lat: newLatencies(), ledger: map[string]entry{}, provSem: make(chan struct{}, 1)}, nil
}

// ID is the provider id.
func (p *Provider) ID() string { return ProviderID }

// Class is `local`: the machine is the operator's own.
func (p *Provider) Class() string { return burst.ClassLocal }

// Observe records a measured provisioning latency for the level of the resource: the time from the
// granted slot to the first command running in its sandbox. Estimate returns the smoothed figure.
func (p *Provider) Observe(id string, d time.Duration) {
	p.mu.Lock()
	e, ok := p.ledger[id]
	p.mu.Unlock()
	if ok {
		p.lat.observe(e.level, d)
	}
}

// Attempt is the isolation attempt a runner must use for the job of this resource: the adapters name
// the sandbox after it, so Teardown(res.ID) and the leak sweep find exactly that sandbox.
func Attempt(res burst.Resource) isolation.Attempt {
	return isolation.Attempt{ID: res.ID, CoordinatorID: res.Labels[coordKey]}
}

// Level returns the isolation level the reservation was granted at ("" for an unknown id).
func (p *Provider) Level(id string) model.Isolation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ledger[id].level
}

// Estimate is local arithmetic: the smoothed latency of the level last offered (or the platform's
// slowest level before any probe) and cost class `local`. It never calls the runtime.
func (p *Provider) Estimate(_ context.Context, s burst.Spec) (burst.Estimate, error) {
	if err := p.platformOK(s); err != nil {
		return burst.Estimate{}, err
	}
	p.mu.Lock()
	lvl := p.level
	p.mu.Unlock()
	if lvl == "" {
		lvl = p.slowestLevel()
	}
	ms, _ := p.lat.get(lvl)
	return burst.Estimate{LatencyMs: ms, Class: burst.ClassLocal}, nil
}

func (p *Provider) slowestLevel() model.Isolation {
	if strings.HasPrefix(p.opts.Backend.Platform(), "darwin/") {
		return vmLevel
	}
	return sandboxLevel
}

func (p *Provider) platformOK(s burst.Spec) error {
	if s.Platform != "" && s.Platform != p.opts.Backend.Platform() {
		return unavailable(ReasonPlatform, nil, "this host offers %s, the job needs %s", p.opts.Backend.Platform(), s.Platform)
	}
	return nil
}

// Provision reserves one sandbox slot for the intent. The id is the intent token, so a second call for
// the same intent returns the same resource (and adopts a sandbox the runtime already holds for it).
func (p *Provider) Provision(ctx context.Context, in burst.Intent) (burst.Resource, error) {
	if err := checkIntent(in); err != nil {
		return burst.Resource{}, err
	}
	if err := ctx.Err(); err != nil {
		return burst.Resource{}, err
	}
	select {
	case p.provSem <- struct{}{}:
		defer func() { <-p.provSem }()
	case <-ctx.Done():
		return burst.Resource{}, ctx.Err()
	}
	id := idPrefix + in.Token
	p.mu.Lock()
	if e, ok := p.ledger[id]; ok {
		p.mu.Unlock()
		return cloneRes(e.res), nil
	}
	p.mu.Unlock()
	if err := p.platformOK(in.Spec); err != nil {
		return burst.Resource{}, err
	}
	off, found, err := p.probe(ctx)
	if err != nil {
		return burst.Resource{}, err
	}
	if f, ok := adoptable(found, in); ok { // the runtime already holds this intent's sandbox
		return p.record(id, off.Level, resFromFound(f)), nil
	}
	used := p.used(off, found)
	limit := p.limit(off, in.Spec, used)
	if used >= limit && p.reclaim(ctx) { // orphans of a dead coordinator may be what fills the host
		if off, found, err = p.probe(ctx); err != nil {
			return burst.Resource{}, err
		}
		used = p.used(off, found)
		limit = p.limit(off, in.Spec, used)
	}
	if used >= limit {
		return burst.Resource{}, fmt.Errorf("%w: %d of %d local slots are in use", burst.ErrQuota, used, limit)
	}
	att := isolation.Attempt{ID: id, CoordinatorID: in.Labels[coordKey]}
	if err := p.opts.Backend.Prepare(ctx, att); err != nil {
		if errors.Is(err, burst.ErrQuota) { // a slot taken by a non-burst job since the check above
			return burst.Resource{}, err
		}
		return burst.Resource{}, classify(ctx, err)
	}
	res := burst.Resource{ID: id, Name: namePrefix + id, Labels: maps.Clone(in.Labels), CreatedAt: p.opts.Now()}
	return p.record(id, off.Level, res), nil
}

// probe asks the backend and remembers the level it offered for Estimate.
func (p *Provider) probe(ctx context.Context) (Offer, []Found, error) {
	off, err := p.opts.Backend.Probe(ctx)
	if err != nil {
		return Offer{}, nil, classify(ctx, err)
	}
	found, err := p.opts.Backend.Leftovers(ctx)
	if err != nil {
		return Offer{}, nil, classify(ctx, err)
	}
	p.mu.Lock()
	p.level = off.Level
	p.mu.Unlock()
	return off, found, nil
}

func (p *Provider) record(id string, level model.Isolation, r burst.Resource) burst.Resource {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ledger[id] = entry{res: r, level: level}
	return cloneRes(r)
}

func (p *Provider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ledger)
}

// used is the slots taken now: every reservation, plus every sandbox the runtime holds that no reservation of
// this provider explains (a leak, another process's job, a VM Apple's limit counts).
func (p *Provider) used(off Offer, found []Found) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := map[string]bool{}
	for _, f := range found {
		if !strings.HasSuffix(f.ID, cleanSuffix) { // a job's cleanup helper is not a second slot
			seen[f.ID] = true
		}
	}
	for _, id := range off.Running {
		seen[id] = true
	}
	n := len(p.ledger)
	for id := range seen {
		if _, ok := p.ledger[id]; !ok {
			n++
		}
	}
	return n
}

// Teardown removes the sandbox of the resource and forgets the reservation. An id this provider cannot
// have issued is not found, which is success; a runtime that cannot be reached is an E698 to retry.
func (p *Provider) Teardown(ctx context.Context, id string) error {
	if !idRe.MatchString(id) {
		return nil
	}
	p.mu.Lock()
	coord := p.ledger[id].res.Labels[coordKey]
	p.mu.Unlock()
	if err := p.opts.Backend.Teardown(ctx, isolation.Attempt{ID: id, CoordinatorID: coord}); err != nil {
		return classify(ctx, err)
	}
	p.mu.Lock()
	delete(p.ledger, id)
	p.mu.Unlock()
	return nil
}

// List returns the reservations and the runtime's labelled sandboxes that carry every requested label.
func (p *Provider) List(ctx context.Context, labels map[string]string) ([]burst.Resource, error) {
	p.reclaim(ctx) // the sweeper lists to find leaks: let a backend that sweeps by its own proof do it now
	found, err := p.opts.Backend.Leftovers(ctx)
	if err != nil {
		return nil, classify(ctx, err)
	}
	byID := map[string]burst.Resource{}
	for _, f := range found {
		byID[f.ID] = resFromFound(f)
	}
	p.mu.Lock()
	for id, e := range p.ledger { // the reservation's own record wins: it carries the intent label
		byID[id] = e.res
	}
	p.mu.Unlock()
	var out []burst.Resource
	for _, r := range byID {
		if burst.Matches(r.Labels, labels) {
			out = append(out, cloneRes(r))
		}
	}
	return out, nil
}
