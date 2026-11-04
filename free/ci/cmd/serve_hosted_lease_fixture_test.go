package main

// Purpose: the fixtures of the hosted lease wiring test (D-1297): a fake hosted provider with a short lease
//          profile and a run that finishes a fixed time after dispatch, its runner capability and evidence, the
//          carrier and sink the production transport adapter stands for, the poll observer and a trust policy
//          that authorizes hosted execution.
// Inputs:  none.
// Outputs: helpers only.
// Constraints: no network: the fake answers every call; every secret-looking value is synthetic; the provider
//          describes its runner only while the test sets it on.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// pollProvider is the fake provider with a hosted runner, a run that finishes pollRunFor after dispatch, and a
// short lease profile.
type pollProvider struct {
	*fake.Fake
	mu         sync.Mutex
	on         bool
	finishAt   time.Time
	dispatches int
}

func (p *pollProvider) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.on {
		return []model.Capability{}, nil
	}
	return []model.Capability{pollCapability()}, nil
}

func (p *pollProvider) Dispatch(ctx context.Context, a providers.Attempt) (providers.Handle, error) {
	p.mu.Lock()
	p.dispatches++
	p.finishAt = time.Now().Add(pollRunFor)
	p.mu.Unlock()
	return p.Fake.Dispatch(ctx, a)
}

func (p *pollProvider) Liveness(context.Context, providers.Handle) (providers.LiveState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Now().Before(p.finishAt) {
		return providers.Running, nil
	}
	return providers.Finished, nil
}

func (*pollProvider) Health(context.Context) providers.Health { return providers.Health{State: "up"} }

func (*pollProvider) Profile() providers.LeaseProfile {
	return providers.LeaseProfile{TTL: 2 * time.Second, Poll: 100 * time.Millisecond}
}

var (
	pollProv     = &pollProvider{}
	pollProvOnce sync.Once
)

// pollCapability is a hosted runner the fixture job is eligible for.
func pollCapability() model.Capability {
	ptr := func(s string) *string { return &s }
	slots, cpu, mem := 2, float64(8), int64(8192)
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = &cpu, &mem
	c.Availability.Slots.Value = &slots
	c.Identity.ID, c.Identity.Name = "fake-hosted-1", "fake-hosted-1"
	c.Identity.Labels = []string{"arch=amd64"}
	c.Economics.Class = model.Fact[string]{Value: ptr("included"), Confidence: "known", Source: "assigned"}
	c.Trust.Accepts = model.Fact[[]model.TrustClass]{Value: &[]model.TrustClass{"owner"}, Confidence: "known", Source: "assigned"}
	iso := model.Isolation("hosted-disposable")
	c.Trust.Isolation = model.Fact[model.Isolation]{Value: &iso, Confidence: "known", Source: "assigned"}
	yes, own := true, "operator"
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &yes, Confidence: "known", Source: "assigned"}
	c.Identity.Ownership = model.Fact[string]{Value: &own, Confidence: "known", Source: "assigned"}
	c.Trust.Network = model.Fact[[]model.NetworkScope]{Value: &[]model.NetworkScope{"privileged"}, Confidence: "known", Source: "assigned"}
	return c
}

// pollEvidence is the ci.evidence/v1 document the fake runner uploads.
func pollEvidence(a providers.Attempt) []byte {
	b, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: a.Revision, Attempt: &model.AttemptRef{ID: a.ID, N: a.N}, Result: "pass",
		Producer: model.Producer{Kind: "signed-remote", Version: "1"}, Signature: model.Signature{Kind: "runner-ed25519"}})
	return b
}

// pollSink is the carrier's sink: a result moves the attempt to finalizing through the lease engine, as the
// run's remote sink does.
type pollSink struct {
	eng     *lease.Engine
	mu      sync.Mutex
	results []protocol.Result
	errs    []error
}

func (s *pollSink) Result(ctx context.Context, r protocol.Result) error {
	l, err := s.eng.Store.LeaseGet(ctx, r.LeaseID)
	if err == nil && l.State == "leased" {
		err = s.eng.Store.Transition(ctx, l.AttemptID, "leased", "running", store.TransitionDetail{Epoch: l.AttemptEpoch})
	}
	if err == nil {
		err = s.eng.Result(ctx, r.LeaseID, int64(r.Epoch))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	if err != nil {
		s.errs = append(s.errs, err)
	}
	return err
}
func (s *pollSink) got() ([]protocol.Result, []error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.Result(nil), s.results...), append([]error(nil), s.errs...)
}
func (*pollSink) Log(context.Context, protocol.Log) error                 { return nil }
func (*pollSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (*pollSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (*pollSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, context.Canceled
}

// pollCarrier delivers an admitted lease through the transport registry, as the production adapter does.
type pollCarrier struct{ sink transport.Sink }

func (pollCarrier) Known(kind string) bool { _, ok := transport.Lookup(kind); return ok }
func (c pollCarrier) Deliver(ctx context.Context, d dispatch.Delivery) error {
	l := protocol.Lease{LeaseID: d.LeaseID, AttemptID: d.AttemptID, Epoch: uint64(d.Epoch), Token: d.Token, CompatMode: protocol.Compat15, Job: d.Job,
		Checkout: protocol.Checkout{Revision: d.Revision}, Secrets: []protocol.SecretRef{}, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}
	return transport.Deliver(ctx, d.Node, l, c.sink)
}

// pollObserved records the coordinator's hosted polls.
type pollObserved struct {
	mu    sync.Mutex
	ready chan struct{}
	once  sync.Once
	polls map[string][]lease.LiveState
	errs  []error
}

func (o *pollObserved) observe(l store.HostedLease, s lease.LiveState, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.polls[l.LeaseID] = append(o.polls[l.LeaseID], s)
	if err != nil {
		o.errs = append(o.errs, err)
	}
	o.once.Do(func() { close(o.ready) })
}
func (o *pollObserved) of(leaseID string) ([]lease.LiveState, []error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]lease.LiveState(nil), o.polls[leaseID]...), append([]error(nil), o.errs...)
}

// serveHostedTrust is an effective trust policy whose project authorizes hosted execution.
func serveHostedTrust(t *testing.T) trust.Effective {
	t.Helper()
	r, err := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
