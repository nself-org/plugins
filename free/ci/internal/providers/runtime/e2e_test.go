package runtime

// Purpose: the end-to-end and liveness tests of the hosted provider runtime (P7-HOST-16).
// Inputs:  the fake provider, a real store, the real world builder, dispatcher and lease engine, and
//          an injected clock.
// Outputs: pass or fail.
// Constraints: no network; the dispatcher is the only thing that admits an attempt.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
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

// carrier adapts the transport registry to dispatch.Transport the way the production wiring does.
type carrier struct{ sink transport.Sink }

func (carrier) Known(kind string) bool { _, ok := transport.Lookup(kind); return ok }
func (c carrier) Deliver(ctx context.Context, d dispatch.Delivery) error {
	l := protocol.Lease{LeaseID: d.LeaseID, AttemptID: d.AttemptID, Epoch: uint64(d.Epoch), Token: d.Token, CompatMode: protocol.Compat15, Job: d.Job,
		Checkout: protocol.Checkout{Revision: d.Revision}, Secrets: []protocol.SecretRef{}, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}
	return transport.Deliver(ctx, d.Node, l, c.sink)
}

// decisionLog logs every placement decision, so a job that never places says why.
type decisionLog struct{ t *testing.T }

func (l decisionLog) Record(_ context.Context, d dispatch.Decision) {
	l.t.Logf("decision outcome=%s reason=%s reasons=%+v", d.Outcome, d.Reason, d.Explanation.Reasons)
}

func queueAttempt(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe", Revision: testRev, Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: "pipe", Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
}

// TestFakeEndToEnd proves acceptance line 1: with the fake provider registered and a policy fixture
// authorizing hosted execution, the world builder (through the "providers" contributor) offers the
// fake hosted runner, the scheduler places the job on it, dispatch.Dispatcher admits it, the driver
// dispatches, polls, collects, and the result evidence is self-reported, coordinator-recorded and
// carries the provider_attestation. The attempt exists only because the dispatcher admitted it.
func TestFakeEndToEnd(t *testing.T) {
	p := useProvider(t)
	st := openStore(t, nil)
	UseStore(st)
	sink := &recSink{}
	buildWorld := func(ctx context.Context, pl sched.Pipeline) (sched.World, string, error) {
		w, digest, err := world.Build(ctx, world.Deps{Store: st, Policy: func(context.Context, string) (sched.Policy, error) {
			return sched.Policy{Preset: "balanced", Trust: hostedPolicy(t)}, nil
		}}, pl)
		w.Costs = map[string]sched.Cost{"fake-hosted-1": {Class: "included", RateMicro: 1, BillingIncrementMs: 60000}}
		w.Allowances = map[string]sched.Allowance{"fake": {RemainingEstimated: 1_000_000, Known: true, CeilingKnown: true}}
		return w, digest, err
	}
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", World: buildWorld, Transport: carrier{sink}, Poll: time.Millisecond, Sink: decisionLog{t}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	queueAttempt(t, st, "att-1")
	job := sched.Job{Key: "att-1", ID: "att-1", Name: "att-1", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
	a, err := d.Submit(context.Background(), dispatch.Job{AttemptID: "att-1", Sched: job, Revision: testRev, Model: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "5m"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Local || a.RunnerID != "fake-hosted-1" || a.Epoch == 0 {
		t.Fatalf("the job must be admitted on the hosted runner: %+v", a)
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(sink.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := sink.got()
	if len(got) != 1 || got[0].LeaseID != a.LeaseID || got[0].Epoch != uint64(a.Epoch) {
		t.Fatalf("one result for the admitted lease expected, got %+v", got)
	}
	var ev model.Evidence
	if err = json.Unmarshal(got[0].Evidence, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Producer.Kind != "self-reported" || ev.Signature.Kind != "coordinator-recorded" || ev.ProviderAttestation == nil ||
		ev.ProviderAttestation.Provider != "fake" || ev.ProviderAttestation.Nonce != "nonce-att-1" || ev.ProviderAttestation.RunID != "att-1" || ev.ProviderAttestation.VerifiedAt == "" {
		t.Fatalf("hosted evidence: producer=%+v signature=%+v attestation=%+v", ev.Producer, ev.Signature, ev.ProviderAttestation)
	}
	if n, _ := p.counts(); n != 1 {
		t.Fatalf("dispatched %d times, want once", n)
	}
	if in, ok, err := st.DispatchIntentGet(context.Background(), "att-1"); err != nil || !ok || in.Provider != "fake" || in.Nonce != "nonce-att-1" {
		t.Fatalf("the dispatch intent must be durable: %+v %v %v", in, ok, err)
	}
}

// grantLease creates attempt id with a running lease l-id (epoch 1<<32|1, initial life ttl) the way the lease engine tests do.
func grantLease(t *testing.T, e *lease.Engine, id string, ttl time.Duration) {
	t.Helper()
	ctx := context.Background()
	s := e.Store
	if err := s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Project: "p", Revision: testRev, Trigger: "local", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	dem := store.Demand{AttemptID: id, LeaseID: "l-" + id, RunnerID: "fake-hosted-1", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: ttl}
	if _, err := e.Grant(ctx, "host", dem, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
}

// TestHostedLiveness proves Epic acceptance 9. A fake run that stays queued for ten minutes of
// injected clock is dispatched once and its lease is renewed by the hosted profile (a second driver
// run for the same attempt, as after a coordinator restart, does not dispatch again); a run that
// vanishes goes lost after the profile TTL and not before.
func TestHostedLiveness(t *testing.T) {
	fixture := func(t *testing.T) (*hostedProv, *clock, *lease.Engine, Book) {
		p := useProvider(t)
		clk := newClock()
		st := openStore(t, clk.Now)
		e := lease.New(st, nil, lease.Config{Clock: clk.Now})
		book := UseStore(st)
		if err := RegisterProfiles(e, book); err != nil {
			t.Fatal(err)
		}
		if err := RegisterProfiles(e, book); err == nil {
			t.Fatal("a second registration for the same engine must be refused by the engine")
		}
		return p, clk, e, book
	}
	t.Run("queued ten minutes is renewed and never re-dispatched", func(t *testing.T) {
		p, clk, e, _ := fixture(t)
		ctx := context.Background()
		start := clk.Now()
		grantLease(t, e, "q1", 2*time.Minute) // far shorter than the ten minutes it will wait: only the hosted renewals keep it alive
		grantLease(t, e, "control", 2*time.Minute)
		node, l := hostedLease("q1")
		p.live = providers.Queued
		polls := 0
		clk.sleep = func(c context.Context, _ time.Duration) {
			if polls++; polls == 30 { // 30 x 20 s = ten minutes queued
				p.live = providers.Finished
				return
			}
			if _, _, err := e.PollDue(c, "fake", "l-q1", "q1", int64(l.Epoch)); err != nil {
				t.Errorf("hosted poll: %v", err)
			}
		}
		defaultDriver.SetClock(clk.Sleep, clk.Now)
		sink := &recSink{}
		if err := defaultDriver.Run(ctx, node, l, sink); err != nil {
			t.Fatal(err)
		}
		if got := clk.Now().Sub(start); got < 10*time.Minute {
			t.Fatalf("the fake run was queued for %v, want at least ten minutes", got)
		}
		if n, err := e.Sweep(ctx); err != nil || n != 1 {
			t.Fatalf("sweep must lose exactly the unrenewed control lease, lost %d (%v)", n, err)
		}
		if state, _, _ := e.Store.AttemptStateEpoch(ctx, "q1"); state != "running" {
			t.Fatalf("the renewed queued run is %q, want running", state)
		}
		if err := defaultDriver.Run(ctx, node, l, sink); err != nil { // restart: same attempt, same handle
			t.Fatal(err)
		}
		if n, _ := p.counts(); n != 1 || len(sink.got()) != 2 {
			t.Fatalf("a queued or restarted run must not be dispatched again: %d dispatches, %d results", n, len(sink.got()))
		}
	})
	t.Run("vanished is lost after the profile TTL and not before", func(t *testing.T) {
		p, clk, e, book := fixture(t)
		ctx := context.Background()
		grantLease(t, e, "v1", 30*time.Minute)
		if err := book.Put(ctx, "v1", providers.Handle{Provider: "fake", RunID: "v1", Nonce: "nonce-v1"}); err != nil {
			t.Fatal(err)
		}
		p.live = providers.Vanished
		if _, err := e.PollHosted(ctx, "fake", "l-v1", "v1", 1<<32|1); err != nil {
			t.Fatal(err)
		}
		ttl := p.Profile().TTL
		clk.advance(ttl - time.Second)
		if n, err := e.Sweep(ctx); err != nil || n != 0 {
			t.Fatalf("a vanished run must not be lost before the profile TTL: %d %v", n, err)
		}
		clk.advance(2 * time.Second)
		if n, err := e.Sweep(ctx); err != nil || n != 1 {
			t.Fatalf("a vanished run must be lost after the profile TTL: %d %v", n, err)
		}
		if state, _, _ := e.Store.AttemptStateEpoch(ctx, "v1"); state != "lost" && state != "retryable" { // an idempotent job goes back to the queue
			t.Fatalf("attempt state after the TTL = %q, want lost or retryable", state)
		}
	})
}

// TestHostedLivenessUnknownAttemptIsVanished: an attempt with no handle (never dispatched) is not kept alive.
func TestHostedLivenessUnknownAttemptIsVanished(t *testing.T) {
	useProvider(t)
	got, err := liveness("fake", prov, NewMemBook())(context.Background(), "nobody")
	if err != nil || got != lease.LiveVanished {
		t.Fatalf("no handle = %q, %v; want vanished", got, err)
	}
}
