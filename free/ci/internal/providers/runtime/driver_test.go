package runtime

// Purpose: tests of the transport driver and the world contributor (P7-HOST-16, acceptance lines 2
// and 3 and the driver's error paths).
// Inputs:  the fake provider with a controllable run state, an injected clock, a recording sink and
//          the real world builder.
// Outputs: pass or fail.
// Constraints: no network; the provider registry has no unregister, so the credential-less provider
//          is registered once.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// noCred is a registered provider whose credential never resolves (no rule is registered for it).
type noCred struct{ *fake.Fake }

func (noCred) ID() string { return "nocred" }

var noCredOnce sync.Once

func codeOf(err error) string {
	var pe *providers.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func buildWorld(t *testing.T) (sched.World, string) {
	t.Helper()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	w, digest, err := world.Build(context.Background(), world.Deps{Store: openStore(t, nil), Clock: func() time.Time { return now },
		Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{Preset: "balanced"}, nil }}, sched.Pipeline{ID: "pipe"})
	if err != nil {
		t.Fatal(err)
	}
	return w, digest
}

// TestWorldContributor proves acceptance line 3: the world digest is stable for the same Describe
// output; a provider whose credential does not resolve contributes no runner and the explanation
// lists E700 under the provider; a provider reporting down keeps its runner, offline and flagged.
func TestWorldContributor(t *testing.T) {
	p := useProvider(t)
	noCredOnce.Do(func() {
		if err := providers.Register(noCred{fake.New(fake.Options{})}); err != nil {
			t.Fatal(err)
		}
	})
	w1, d1 := buildWorld(t)
	w2, d2 := buildWorld(t)
	if d1 != d2 || len(w1.Hosted) != 1 || len(w2.Hosted) != 1 {
		t.Fatalf("digests %s %s, hosted %d %d", d1, d2, len(w1.Hosted), len(w2.Hosted))
	}
	r := w1.Hosted[0]
	if r.ID != "fake-hosted-1" || r.Provider != "fake" || r.Location != "hosted" || r.Capability.Identity.Transport != Kind || !r.Decl.Hosted || r.ProviderDown || !r.Active || r.Avail.Slots != 2 {
		t.Fatalf("the hosted runner: %+v", r)
	}
	var found *Skip
	for _, s := range DefaultContributor().Skipped() {
		if s.Provider == "nocred" {
			found = &s
		}
		if s.Provider == "fake" {
			t.Fatalf("the fake provider has a credential and must not be skipped: %+v", s)
		}
	}
	if found == nil || found.Code != "E700" {
		t.Fatalf("a provider with no credential must be listed with E700: %+v", DefaultContributor().Skipped())
	}
	for _, h := range w1.Hosted {
		if h.Provider == "nocred" {
			t.Fatal("a provider with no credential contributed a runner")
		}
	}

	p.down = true
	w3, d3 := buildWorld(t)
	if d3 == d1 || len(w3.Hosted) != 1 || !w3.Hosted[0].ProviderDown || w3.Hosted[0].Active || *w3.Hosted[0].Capability.Availability.State.Value != "offline" {
		t.Fatalf("a provider that is down keeps an offline runner flagged ProviderDown: %+v", w3.Hosted)
	}
	p.down = false

	t.Setenv("NSELF_V15", "")
	if w4, _ := buildWorld(t); len(w4.Hosted) != 0 {
		t.Fatal("v1.4 mode must contribute no hosted runner")
	}
}

func TestContributorReservations(t *testing.T) {
	useProvider(t)
	ctx := context.Background()
	st := openStore(t, nil)
	DefaultContributor().SetStore(st)
	w := sched.World{}
	if err := DefaultContributor().Fill(ctx, &w); err != nil || len(w.Hosted) != 1 || w.Hosted[0].Avail.Slots != 2 {
		t.Fatalf("fill with a store: %+v %v", w.Hosted, err)
	}
	DefaultContributor().SetTargets(func(context.Context) ([]providers.Query, error) { return nil, errors.New("targets down") })
	if err := DefaultContributor().Fill(ctx, &sched.World{}); err == nil {
		t.Fatal("an unreadable target list must fail the contributor, not hide every runner")
	}
}

func runDriver(ctx context.Context, t *testing.T, id string) (*recSink, *clock, error) {
	t.Helper()
	clk, sink := newClock(), &recSink{}
	defaultDriver.SetClock(clk.Sleep, clk.Now)
	node, l := hostedLease(id)
	err := defaultDriver.Run(ctx, node, l, sink)
	return sink, clk, err
}

func TestDriverPollsUntilFinishedAndRetriesTransientErrors(t *testing.T) {
	p := useProvider(t)
	p.live = providers.Queued
	p.liveErr = providers.NewError(providers.Transient, "E704", "blip", 503, time.Time{})
	clk, sink := newClock(), &recSink{}
	n := 0
	clk.sleep = func(context.Context, time.Duration) {
		switch n++; n {
		case 2:
			p.liveErr = nil
		case 4:
			p.live = providers.Running
		case 6:
			p.live = providers.Finished
		}
	}
	defaultDriver.SetClock(clk.Sleep, clk.Now)
	node, l := hostedLease("t1")
	if err := defaultDriver.Run(context.Background(), node, l, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.got()) != 1 || n != 6 || clk.Now().Sub(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)) != 6*20*time.Second {
		t.Fatalf("results=%d sleeps=%d elapsed=%v (the profile's 20 s poll)", len(sink.got()), n, clk.Now())
	}
}

func TestDriverErrorPaths(t *testing.T) {
	t.Run("auth failure ends the run at once", func(t *testing.T) {
		p := useProvider(t)
		p.live, p.liveErr = providers.Queued, providers.NewError(providers.Auth, "E701", "scope", 403, time.Time{})
		sink, _, err := runDriver(context.Background(), t, "e1")
		if codeOf(err) != "E701" || len(sink.got()) != 0 {
			t.Fatalf("err=%v results=%d", err, len(sink.got()))
		}
	})
	t.Run("a vanished run is E704 with no result", func(t *testing.T) {
		p := useProvider(t)
		p.live = providers.Vanished
		sink, _, err := runDriver(context.Background(), t, "e2")
		if codeOf(err) != "E704" || len(sink.got()) != 0 {
			t.Fatalf("err=%v results=%d", err, len(sink.got()))
		}
	})
	t.Run("an unknown state is refused", func(t *testing.T) {
		p := useProvider(t)
		p.live = "paused"
		if _, _, err := runDriver(context.Background(), t, "e3"); codeOf(err) != "E704" {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("evidence for another attempt is E705", func(t *testing.T) {
		useProvider(t)
		prov.Fake = fake.New(fake.Options{Evidence: func(a providers.Attempt) []byte { a.ID = "someone-else"; return evidenceFor(a) }})
		sink, _, err := runDriver(context.Background(), t, "e4")
		if codeOf(err) != "E705" || len(sink.got()) != 0 {
			t.Fatalf("err=%v results=%d", err, len(sink.got()))
		}
	})
	t.Run("evidence for another revision is E705", func(t *testing.T) {
		useProvider(t)
		prov.Fake = fake.New(fake.Options{Evidence: func(a providers.Attempt) []byte { a.Revision = strings.Repeat("a", 40); return evidenceFor(a) }})
		if _, _, err := runDriver(context.Background(), t, "e5"); codeOf(err) != "E705" {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("an attestation for another run or nonce is E705", func(t *testing.T) {
		p := useProvider(t)
		for i, mutate := range []func(*providers.Collected){
			func(c *providers.Collected) { c.Attestation.Nonce = "forged" },
			func(c *providers.Collected) { c.Attestation.RunID = "other-run" },
			func(c *providers.Collected) { c.Attestation.Provider = "github" },
		} {
			p.mutate = mutate
			if _, _, err := runDriver(context.Background(), t, "e6-"+string(rune('a'+i))); codeOf(err) != "E705" {
				t.Fatalf("mutation %d: err=%v", i, err)
			}
		}
	})
	t.Run("a document that is not evidence is E705", func(t *testing.T) {
		useProvider(t)
		prov.Fake = fake.New(fake.Options{Evidence: func(providers.Attempt) []byte { return []byte(`{"attempt":"x"}`) }})
		if _, _, err := runDriver(context.Background(), t, "e7"); codeOf(err) != "E705" {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("an unregistered provider is E700", func(t *testing.T) {
		useProvider(t)
		node, l := hostedLease("e8")
		node.Identity.Provider = "ghost"
		if err := defaultDriver.Run(context.Background(), node, l, &recSink{}); codeOf(err) != "E700" {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("a lease with no revision is never dispatched", func(t *testing.T) {
		p := useProvider(t)
		node, l := hostedLease("e9")
		l.Checkout.Revision = " "
		if err := defaultDriver.Run(context.Background(), node, l, &recSink{}); codeOf(err) != "E708" {
			t.Fatalf("err=%v", err)
		}
		if n, _ := p.counts(); n != 0 {
			t.Fatalf("dispatched %d times with no revision", n)
		}
	})
	t.Run("a handle from another provider is refused", func(t *testing.T) {
		useProvider(t)
		if _, err := defaultDriver.Book().Put(context.Background(), "e10", Entry{Handle: providers.Handle{Provider: "github", RunID: "1"}, Epoch: 1<<32 | 1}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runDriver(context.Background(), t, "e10"); codeOf(err) != "E708" {
			t.Fatalf("err=%v", err)
		}
	})
}

// TestDriverCancelsTheRunWhenTheLeaseIsCancelled: a cancelled context cancels the run at the provider once.
func TestDriverCancelsTheRunWhenTheLeaseIsCancelled(t *testing.T) {
	p := useProvider(t)
	p.live = providers.Running
	ctx, cancel := context.WithCancel(context.Background())
	clk, sink := newClock(), &recSink{}
	n := 0
	clk.sleep = func(context.Context, time.Duration) {
		if n++; n == 3 {
			cancel()
		}
	}
	defaultDriver.SetClock(clk.Sleep, clk.Now)
	node, l := hostedLease("c1")
	err := defaultDriver.Run(ctx, node, l, sink)
	if !errors.Is(err, context.Canceled) || len(sink.got()) != 0 {
		t.Fatalf("err=%v results=%d", err, len(sink.got()))
	}
	if d, c := p.counts(); d != 1 || c != 1 {
		t.Fatalf("dispatches=%d cancels=%d, want 1 and 1", d, c)
	}
}

func TestDriverFactsAndSpecReachTheDispatcher(t *testing.T) {
	p := useProvider(t)
	var got providers.Attempt
	prov.Fake = fake.New(fake.Options{Evidence: func(a providers.Attempt) []byte { got = a; return evidenceFor(a) }})
	defaultDriver.SetFacts(func(context.Context, model.Capability, protocol.Lease) (AttemptFacts, error) {
		return AttemptFacts{N: 3, Project: "o", Repo: "o/r", FetchRef: "refs/pull/1/head", Trust: "owner", Label: "hosted", TimeoutMs: 1234}, nil
	})
	if _, _, err := runDriver(context.Background(), t, "f1"); err != nil {
		t.Fatal(err)
	}
	if got.Revision != testRev || got.Repo != "o/r" || got.N != 3 || got.FetchRef != "refs/pull/1/head" || got.CompatMode != "v1.5" || got.TimeoutMs != 1234 ||
		!strings.HasPrefix(got.SpecDigest, "sha256:") || !strings.Contains(string(got.Spec), `"run":["true"]`) {
		t.Fatalf("attempt handed to the dispatcher: %+v", got)
	}
	if n, _ := p.counts(); n != 1 {
		t.Fatalf("dispatches %d", n)
	}
}
