package runtime

// Purpose: shared fixtures of the runtime tests: one process-wide fake provider (the provider and
// source registries have no unregister), a hosted capability, a valid evidence document, an injected
// clock and a recording sink.
// Inputs:  none.
// Outputs: helpers only.
// Constraints: no network (fake provider and httptest only); every secret-looking value is synthetic;
//          tests that change the shared provider restore it with t.Cleanup.

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const testRev = "0123456789abcdef0123456789abcdef01234567"

// hostedProv is the fake provider with a hosted capability, a controllable run state and a
// dispatch counter. It is both a provider and a trigger source named "fake".
type hostedProv struct {
	*fake.Fake
	mu         sync.Mutex
	live       providers.LiveState
	liveErr    error
	dispatches int
	cancels    int
	down       bool
	slots      int
	mutate     func(*providers.Collected)
}

func (h *hostedProv) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return []model.Capability{hostedCapability("fake-hosted-1", h.slots)}, nil
}
func (h *hostedProv) Dispatch(ctx context.Context, a providers.Attempt) (providers.Handle, error) {
	h.mu.Lock()
	h.dispatches++
	h.mu.Unlock()
	return h.Fake.Dispatch(ctx, a)
}
func (h *hostedProv) Cancel(ctx context.Context, handle providers.Handle) error {
	h.mu.Lock()
	h.cancels++
	h.mu.Unlock()
	return h.Fake.Cancel(ctx, handle)
}
func (h *hostedProv) Liveness(context.Context, providers.Handle) (providers.LiveState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.live, h.liveErr
}
func (h *hostedProv) Collect(ctx context.Context, handle providers.Handle) (providers.Collected, error) {
	c, err := h.Fake.Collect(ctx, handle)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mutate != nil {
		h.mutate(&c)
	}
	return c, err
}
func (h *hostedProv) Health(context.Context) providers.Health {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.down {
		return providers.Health{State: "down", Reason: "outage"}
	}
	return providers.Health{State: "up"}
}
func (h *hostedProv) Match(r *http.Request) bool { return r.Header.Get("X-Test-Source") == "fake" }
func (h *hostedProv) Verify(ctx context.Context, r *http.Request, body []byte) (model.TriggerFacts, error) {
	f, err := h.Fake.Verify(ctx, r, body)
	f.Repo, f.Revision, f.DeliveryID = "o/r", testRev, r.Header.Get("X-Test-Delivery")
	if r.Header.Get("X-Test-Kind") != "" {
		f.Kind = model.TriggerKind(r.Header.Get("X-Test-Kind"))
	}
	return f, err
}
func (h *hostedProv) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dispatches, h.cancels
}

var (
	prov     = &hostedProv{}
	provOnce sync.Once
)

// useProvider registers the shared fake provider once, resets its behaviour for this test and makes
// every test see a fresh handle book, store hookup and no V15 leftovers.
func useProvider(t *testing.T) *hostedProv {
	t.Helper()
	provOnce.Do(func() {
		prov.Fake = fake.New(fake.Options{Evidence: evidenceFor})
		for _, err := range []error{providers.Register(prov), providers.RegisterSource(prov), RegisterCredential("fake", func(context.Context) rest.AuthFunc {
			return func(context.Context) (string, error) { return "synthetic-test-token", nil }
		})} {
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	prov.mu.Lock()
	prov.Fake = fake.New(fake.Options{Evidence: evidenceFor})
	prov.live, prov.liveErr, prov.dispatches, prov.cancels, prov.down, prov.slots, prov.mutate = providers.Finished, nil, 0, 0, false, 2, nil
	prov.mu.Unlock()
	t.Setenv("NSELF_V15", "1")
	defaultDriver.SetBook(NewMemBook())
	defaultDriver.SetFacts(testFacts)
	defaultDriver.SetClock(nil, nil)
	defaultContributor.SetStore(nil)
	defaultContributor.SetTargets(nil)
	defaultContributor.ForgetCredentials()
	UsePolicy(nil)
	t.Cleanup(func() {
		defaultDriver.SetBook(NewMemBook())
		defaultDriver.SetFacts(nil)
		defaultDriver.SetClock(nil, nil)
		defaultContributor.SetStore(nil)
		UsePolicy(nil)
	})
	return prov
}

// testFacts stands in for the P7-HOST-03 facts source.
func testFacts(context.Context, model.Capability, protocol.Lease) (AttemptFacts, error) {
	return AttemptFacts{N: 1, Project: "o", Repo: "o/r", TimeoutMs: 300000}, nil
}

func ptr[T any](v T) *T { return &v }

// hostedCapability is a runner every fixture job is eligible for.
func hostedCapability(id string, slots int) model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Availability.Slots.Value = &slots
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.Labels = []string{"arch=amd64"}
	c.Economics.Class = model.Fact[string]{Value: ptr("included"), Confidence: "known", Source: "assigned"}
	c.Trust.Accepts = model.Fact[[]model.TrustClass]{Value: &[]model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Confidence: "known", Source: "assigned"}
	c.Trust.Isolation = model.Fact[model.Isolation]{Value: ptr(model.Isolation("hosted-disposable")), Confidence: "known", Source: "assigned"}
	c.Separation.UIDSeparation = model.Fact[bool]{Value: ptr(true), Confidence: "known", Source: "assigned"}
	c.Identity.Ownership = model.Fact[string]{Value: ptr("operator"), Confidence: "known", Source: "assigned"}
	c.Trust.Network = model.Fact[[]model.NetworkScope]{Value: &[]model.NetworkScope{"privileged"}, Confidence: "known", Source: "assigned"}
	return c
}

// evidenceFor is a ci.evidence/v1 document for the attempt, as a provider's runner would upload it.
func evidenceFor(a providers.Attempt) []byte {
	b, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: a.Revision, Attempt: &model.AttemptRef{ID: a.ID, N: a.N}, Result: "pass",
		Producer: model.Producer{Kind: "signed-remote", Version: "1"}, Signature: model.Signature{Kind: "runner-ed25519"}})
	return b
}

// clock is an injected time source whose Sleep advances it and runs a hook.
type clock struct {
	mu    sync.Mutex
	t     time.Time
	sleep func(context.Context, time.Duration)
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)} }
func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
func (c *clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.advance(d)
	if c.sleep != nil {
		c.sleep(ctx, d)
	}
	return nil
}

// recSink records the results a driver hands over.
type recSink struct {
	mu      sync.Mutex
	results []protocol.Result
}

func (s *recSink) Result(_ context.Context, r protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	return nil
}
func (s *recSink) got() []protocol.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.Result(nil), s.results...)
}
func (*recSink) Log(context.Context, protocol.Log) error                 { return nil }
func (*recSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (*recSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (*recSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, context.Canceled
}

func openStore(t *testing.T, now func() time.Time) *store.Store {
	t.Helper()
	opts := store.Options{}
	if now != nil {
		opts.Clock = now
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// hostedLease is the lease the dispatcher would hand the driver for attempt id.
func hostedLease(id string) (model.Capability, protocol.Lease) {
	node := hostedCapability("fake-hosted-1", 2)
	node.Identity.Provider, node.Identity.Transport = "fake", Kind
	return node, protocol.Lease{LeaseID: "l-" + id, AttemptID: id, Epoch: 1<<32 | 1, CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "5m"}, Checkout: protocol.Checkout{Revision: testRev}}
}
