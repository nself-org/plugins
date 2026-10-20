package dispatch_test

// Remote and provider delivery through the real transport registry: a fake
// driver per transport kind, a fake preparer, and run.NewTransportAdapter, the
// adapter the production wiring uses. The zero-node test runs the whole
// coordinator with the dispatcher registered.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// hostedPolicy is an effective policy whose project authorizes hosted execution.
func hostedPolicy(t *testing.T) trust.Effective {
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

type delivered struct {
	node  model.Capability
	lease protocol.Lease
}

// fakeDriver records the lease it is run with, and whether a preparer touched it first.
type fakeDriver struct {
	kind string
	got  chan delivered
	err  error
}

func (f *fakeDriver) Kind() string { return f.kind }
func (f *fakeDriver) Run(_ context.Context, node model.Capability, l protocol.Lease, _ transport.Sink) error {
	f.got <- delivered{node, l}
	return f.err
}

type nullSink struct{}

func (nullSink) Log(context.Context, protocol.Log) error                 { return nil }
func (nullSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (nullSink) Result(context.Context, protocol.Result) error           { return nil }
func (nullSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, io.ErrClosedPipe
}
func (nullSink) CacheRef(context.Context, protocol.CacheRef) error { return nil }

var (
	registerOnce sync.Once
	remote       = &fakeDriver{kind: "fake-remote", got: make(chan delivered, 8)}
	provider     = &fakeDriver{kind: "fake-provider", got: make(chan delivered, 8)}
	broken       = &fakeDriver{kind: "fake-broken", got: make(chan delivered, 8), err: errors.New("carrier down")}
)

// registerDrivers installs the fakes and one preparer that stamps the lease, once per process.
func registerDrivers(t *testing.T) {
	t.Helper()
	registerOnce.Do(func() {
		for _, d := range []*fakeDriver{remote, provider, broken} {
			if err := transport.Register(d); err != nil {
				t.Fatal(err)
			}
		}
		if err := transport.RegisterPreparer(func(_ context.Context, _ model.Capability, l *protocol.Lease, _ model.Job) error {
			l.Checkout.BundleDigest = "prepared-before-driver"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func wait(t *testing.T, ch chan delivered) delivered {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(20 * time.Second):
		t.Fatal("the driver was never run")
		return delivered{}
	}
}

func newDispatcher(t *testing.T, cfg dispatch.Config) *dispatch.Dispatcher {
	t.Helper()
	cfg.Host = "host"
	cfg.Transport = run.NewTransportAdapter(nullSink{})
	cfg.Poll = time.Millisecond
	d, err := dispatch.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// TestDispatchRemoteDelivery: a remote placement is delivered through the
// driver registered for its transport kind, after the preparers, carrying the
// fencing epoch and the token that was never stored.
func TestDispatchRemoteDelivery(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-a", "lan", 2)
	node.Capability.Identity.Transport = "fake-remote"
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node)})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	a, err := d.Submit(context.Background(), dispatch.QueuedJob(id))
	if err != nil {
		t.Fatal(err)
	}
	if a.Local || a.RunnerID != "node-a" || a.Epoch == 0 || a.Token == "" {
		t.Fatalf("remote admission: %+v", a)
	}
	got := wait(t, remote.got)
	if got.lease.AttemptID != id || got.lease.LeaseID != a.LeaseID || got.lease.Epoch != uint64(a.Epoch) || got.lease.Token != a.Token || got.lease.CompatMode != protocol.Compat15 {
		t.Fatalf("lease handed to the driver: %+v vs %+v", got.lease, a)
	}
	if got.lease.Checkout.BundleDigest != "prepared-before-driver" {
		t.Fatal("the preparer did not run before the driver")
	}
	if got.node.Identity.ID != "node-a" {
		t.Fatalf("driver ran for node %q", got.node.Identity.ID)
	}
	l, err := st.LeaseGet(context.Background(), a.LeaseID)
	if err != nil || l.Epoch != a.Epoch || l.RunnerID != "node-a" {
		t.Fatalf("stored lease: %+v %v", l, err)
	}
}

// loopback is the node's carrier: it hands the lease the dispatcher delivered to the session once, then blocks.
type loopback struct {
	lease chan protocol.Message
	done  chan struct{}
}

func (l *loopback) Send(protocol.Message) error { return nil }
func (l *loopback) Recv() (protocol.Message, error) {
	select {
	case m := <-l.lease:
		return m, nil
	case <-l.done:
		return protocol.Message{}, io.EOF
	}
}
func (l *loopback) Close() error { close(l.done); return nil }

// secretJob queues an attempt of a job that declares one project secret.
func secretJob(t *testing.T, id string) dispatch.Job {
	t.Helper()
	j := dispatch.QueuedJob(id)
	j.Model = model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/API_TOKEN"}}
	j.Sched.SecretClasses = []model.SecretClass{"project"}
	return j
}

// TestDispatchLeaseCarriesGatedSecretRefs: the dispatcher puts the job's gated references and the class it was
// placed at into the lease of a secrets-eligible node, and the node's resolver receives both (D-1373). The
// coordinator side holds no value anywhere on the path.
func TestDispatchLeaseCarriesGatedSecretRefs(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-s", "lan", 2)
	node.Capability.Identity.Transport = "fake-remote"
	node.Capability.SecretsEligible = true
	node.Decl.SecretClasses = []model.SecretClass{"project"}
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node)})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	a, err := d.Submit(context.Background(), secretJob(t, id))
	if err != nil {
		t.Fatal(err)
	}
	got := wait(t, remote.got).lease
	if len(got.Secrets) != 1 || got.Secrets[0].Ref != "project/API_TOKEN" || got.Secrets[0].Class != "project" || got.Trust != "owner" || got.LeaseID != a.LeaseID {
		t.Fatalf("lease secrets = %+v trust %q", got.Secrets, got.Trust)
	}
	const canary = "canary-value-61c0"
	var sawAttempt string
	var sawTrust model.TrustClass
	var sawRefs []string
	car := &loopback{lease: make(chan protocol.Message, 1), done: make(chan struct{})}
	node1 := agent.WithNodeSecrets(car, func(_ context.Context, attempt string, class model.TrustClass, refs []string) (map[string]string, error) {
		sawAttempt, sawTrust, sawRefs = attempt, class, refs
		return map[string]string{"API_TOKEN": canary}, nil
	}, nil)
	defer node1.Close()
	ep := got.Epoch
	car.lease <- protocol.Message{V: 1, Type: "lease", LeaseID: got.LeaseID, Epoch: &ep, Body: &got}
	if m, err := node1.Recv(); err != nil || m.Type != "lease" {
		t.Fatalf("first frame %v %v", m.Type, err)
	}
	m, err := node1.Recv()
	if err != nil || m.Type != "secrets" {
		t.Fatalf("second frame %v %v", m.Type, err)
	}
	if v := m.Body.(*protocol.Secrets).Values; len(v) != 1 || v[0].Ref != "project/API_TOKEN" || v[0].Value != canary {
		t.Fatalf("values = %+v", v)
	}
	if sawAttempt != id || sawTrust != "owner" || len(sawRefs) != 1 || sawRefs[0] != "project/API_TOKEN" {
		t.Fatalf("resolver saw attempt %q trust %q refs %v", sawAttempt, sawTrust, sawRefs)
	}
	blob, _ := json.Marshal(got)
	if strings.Contains(string(blob), canary) {
		t.Fatal("the delivered lease holds a secret value")
	}
}

// TestDispatchLeaseWithoutSecretRefsForAnIneligibleNode: a node that is not secrets-eligible gets no references.
func TestDispatchLeaseWithoutSecretRefsForAnIneligibleNode(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-i", "lan", 2)
	node.Capability.Identity.Transport = "fake-remote"
	node.Decl.SecretClasses = []model.SecretClass{"project"}
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node)})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	if _, err := d.Submit(context.Background(), secretJob(t, id)); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, remote.got).lease; len(got.Secrets) != 0 || got.Trust != "" {
		t.Fatalf("an ineligible node got secrets %+v trust %q", got.Secrets, got.Trust)
	}
}

// TestDispatchUnknownTransport: a runner whose transport kind has no driver
// never receives a lease; the job stays queued with node.protocol.
func TestDispatchUnknownTransport(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-b", "lan", 2)
	node.Capability.Identity.Transport = "no-such-kind"
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node)})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := d.Submit(ctx, dispatch.QueuedJob(id)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("submit: %v", err)
	}
	if reason, _, ok, _ := st.Refusal(context.Background(), id); !ok || reason != "node.protocol" {
		t.Fatalf("reason %q", reason)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "queued" {
		t.Fatalf("attempt is %s", state)
	}
	snap, _ := st.WorldSnapshot(context.Background())
	if len(snap.Active) != 0 {
		t.Fatal("a lease exists for an undeliverable placement")
	}
}

// TestDispatchProviderLease: a hosted provider lease goes through
// transport.Deliver too, and a registered `release` admission step (as
// P7-CACHE-05 registers it) runs inside Admit.
func TestDispatchProviderLease(t *testing.T) {
	registerDrivers(t)
	dispatch.ResetSteps("limits")
	t.Cleanup(func() { dispatch.ResetSteps("limits") })
	var released int
	if err := dispatch.RegisterStep("release", func(context.Context, dispatch.View) error { released++; return nil }); err != nil {
		t.Fatal(err)
	}
	st := dispatch.OpenStore(t)
	hosted := dispatch.SchedRunner("hosted-1", "hosted", 1)
	hosted.Capability.Identity.Transport, hosted.Provider, hosted.Class = "fake-provider", "github", "included"
	hosted.Decl.Hosted, hosted.Decl.Isolation = true, "hosted-disposable"
	d := newDispatcher(t, dispatch.Config{Store: st, World: func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
		w, digest, err := dispatch.LiveWorld(st)(ctx, p)
		w.Hosted = append(w.Hosted, hosted)
		w.Policy.Trust = hostedPolicy(t)
		w.Costs = map[string]sched.Cost{"hosted-1": {Class: "included", RateMicro: 1, BillingIncrementMs: 60000}}
		w.Allowances = map[string]sched.Allowance{"github": {RemainingEstimated: 1000000, Known: true, CeilingKnown: true}}
		return w, digest, err
	}})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	j := dispatch.QueuedJob(id)
	a, err := d.Submit(context.Background(), j)
	if err != nil {
		t.Fatal(err)
	}
	if a.Local || a.RunnerID != "hosted-1" || released != 1 {
		t.Fatalf("provider admission: %+v (release step ran %d times)", a, released)
	}
	got := wait(t, provider.got)
	if got.lease.Checkout.BundleDigest != "prepared-before-driver" || got.node.Identity.ID != "hosted-1" {
		t.Fatalf("provider delivery: %+v", got)
	}
}

// TestDispatchDeliveryFails: a carrier that fails marks the attempt lost.
func TestDispatchDeliveryFails(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-c", "lan", 1)
	node.Capability.Identity.Transport = "fake-broken"
	failed := make(chan error, 1)
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node), DeliveryError: func(a dispatch.Admitted, err error) {
		if a.Token != "" {
			err = errors.New("the lease token reached the failure callback")
		}
		failed <- err
	}})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	if _, err := d.Submit(context.Background(), dispatch.QueuedJob(id)); err != nil {
		t.Fatal(err)
	}
	wait(t, broken.got)
	select {
	case err := <-failed:
		if err == nil || err.Error() == "the lease token reached the failure callback" {
			t.Fatalf("delivery error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("delivery failure not reported")
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "lost" {
		t.Fatalf("attempt is %s, want lost", state)
	}
}

// TestDispatchRaisesDeliveredIsolationToTheFloor (A01): an untrusted job declared at `process` is placed on a
// node that offers more, and the lease the driver receives carries the trust floor, not the declared level.
func TestDispatchRaisesDeliveredIsolationToTheFloor(t *testing.T) {
	registerDrivers(t)
	st := dispatch.OpenStore(t)
	node := dispatch.SchedRunner("node-floor", "lan", 2)
	node.Capability.Identity.Transport = "fake-remote"
	d := newDispatcher(t, dispatch.Config{Store: st, World: dispatch.LiveWorld(st, node)})
	id := dispatch.QueueAttempt(t, st, "pipe", "proj", "job")
	j := dispatch.QueuedJob(id)
	j.Sched.Trust, j.Sched.Network, j.Sched.PersonalOwnRun = "untrusted", "restricted", false
	j.Model = model.Job{Isolation: "process", Run: []string{"true"}}
	if _, err := d.Submit(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	lease := wait(t, remote.got).lease
	if lease.Job.Isolation != "sandboxed-container" || lease.NetworkScope != "restricted" {
		t.Fatalf("delivered isolation=%s network=%s, want sandboxed-container restricted", lease.Job.Isolation, lease.NetworkScope)
	}
}
