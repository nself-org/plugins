package local

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/conformance"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

const (
	coord = "01J00000000000000000COORDA"
	tok1  = "01J0000000000000000000TOK1"
	tok2  = "01J0000000000000000000TOK2"
)

func intentFor(token string) burst.Intent {
	return burst.Intent{Token: token, Spec: burst.Spec{Platform: "linux/amd64", CPU: 1, MemMB: 512, TimeoutMs: 600000}, Labels: burst.IntentLabels(coord, token)}
}

func TestConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Subject {
		fb := newFake()
		return conformance.Subject{
			Provider: newProvider(t, fb, nil),
			Foreign: func(labels map[string]string) string {
				id := fmt.Sprintf("burst-foreign%d", len(fb.boxes))
				fb.add(Found{ID: id, Labels: labels, CreatedAt: epoch})
				return id
			},
			Outage: fb.setDown,
		}
	})
}

func TestConformanceKitCatchesABrokenProvider(t *testing.T) {
	// a provider that never reports CreatedAt must fail the kit: proves the kit runs against this provider type
	fb := newFake()
	p := newProvider(t, fb, func(o *Options) { o.Now = func() time.Time { return time.Time{} } })
	v := conformance.Violations(t, func(*testing.T) conformance.Subject {
		return conformance.Subject{Provider: p, Outage: fb.setDown}
	})
	if len(v) == 0 {
		t.Fatal("a provider with a zero CreatedAt passed the conformance kit")
	}
}

func TestClassAndID(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	if p.Class() != burst.ClassLocal || p.ID() != ProviderID {
		t.Fatalf("class %q id %q", p.Class(), p.ID())
	}
}

func TestProvisionIdentity(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	r, err := p.Provision(context.Background(), intentFor(tok1))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "burst-"+tok1 || r.Name != "nself-ci-burst-"+tok1 {
		t.Fatalf("id %q name %q", r.ID, r.Name)
	}
	if a := Attempt(r); a.ID != r.ID || a.CoordinatorID != coord {
		t.Fatalf("attempt %+v", a)
	}
	r.Labels[roleKey] = "tampered"
	again, _ := p.Provision(context.Background(), intentFor(tok1))
	if again.Labels[roleKey] != roleValue {
		t.Fatal("a caller edited the ledger through the returned labels")
	}
}

func TestBadIntentsAreRefused(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	mut := func(f func(*burst.Intent)) burst.Intent {
		in := intentFor(tok1)
		in.Labels = map[string]string{}
		for k, v := range intentFor(tok1).Labels {
			in.Labels[k] = v
		}
		f(&in)
		return in
	}
	for name, in := range map[string]burst.Intent{
		"empty token":      mut(func(i *burst.Intent) { i.Token = "" }),
		"slash in token":   mut(func(i *burst.Intent) { i.Token = "a/b"; i.Labels[intentKey] = "a/b" }),
		"dash in token":    mut(func(i *burst.Intent) { i.Token = "a-b"; i.Labels[intentKey] = "a-b" }),
		"label mismatch":   mut(func(i *burst.Intent) { i.Labels[intentKey] = "other" }),
		"no role":          mut(func(i *burst.Intent) { delete(i.Labels, roleKey) }),
		"no coordinator":   mut(func(i *burst.Intent) { delete(i.Labels, coordKey) }),
		"long label value": mut(func(i *burst.Intent) { i.Labels["extra"] = string(make([]byte, 64)) }),
	} {
		if _, err := p.Provision(context.Background(), in); ReasonOf(err) != ReasonBadRequest || CodeOf(err) != "E698" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNoRuntimeOffersNoSlots(t *testing.T) {
	fb := newFake()
	fb.setDown(true)
	p := newProvider(t, fb, nil)
	_, err := p.Provision(context.Background(), intentFor(tok1))
	if CodeOf(err) != "E698" || ReasonOf(err) != ReasonNoRuntime || !errors.Is(err, burst.ErrUnavailable) {
		t.Fatalf("want E698 no_runtime that is burst.ErrUnavailable, got %v", err)
	}
	if got, _ := p.List(context.Background(), nil); len(got) != 0 {
		t.Fatalf("slots offered: %+v", got)
	}
	if len(fb.prepared) != 0 {
		t.Fatal("an adapter was prepared although no runtime exists")
	}
}

func TestPlatformMismatchIsE698(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	in := intentFor(tok1)
	in.Spec.Platform = "darwin/arm64"
	if _, err := p.Provision(context.Background(), in); ReasonOf(err) != ReasonPlatform {
		t.Fatalf("provision: %v", err)
	}
	if _, err := p.Estimate(context.Background(), in.Spec); ReasonOf(err) != ReasonPlatform {
		t.Fatalf("estimate: %v", err)
	}
	in.Spec.Platform = ""
	if _, err := p.Provision(context.Background(), in); err != nil {
		t.Fatalf("a spec with no platform fits any host: %v", err)
	}
}

func TestSlotAccounting(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*fakeBackend, *Options)
		slots int
	}{
		{"runtime limit (Apple two VMs)", func(f *fakeBackend, _ *Options) { f.total = 2 }, 2},
		{"runtime limit minus a sandbox nobody reserved", func(f *fakeBackend, _ *Options) { f.total, f.running = 2, []string{"job-from-elsewhere"} }, 1},
		{"max slots", func(_ *fakeBackend, o *Options) { o.MaxSlots = 3 }, 3},
		{"host reservation", func(_ *fakeBackend, o *Options) {
			o.Host = func() (exec.Capacity, reservation.Settings) {
				return exec.Capacity{CPUs: 8, MemoryMB: 64 << 10, FreeMemoryMB: 64 << 10, BatteryPercent: -1}, reservation.Settings{ReservedCPU: 5}
			}
		}, 3},
		{"host reservation holds everything", func(_ *fakeBackend, o *Options) {
			o.Host = func() (exec.Capacity, reservation.Settings) {
				return exec.Capacity{CPUs: 4, MemoryMB: 64 << 10, BatteryPercent: -1}, reservation.Settings{ReservedCPU: 4}
			}
		}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fb := newFake()
			p := newProvider(t, fb, func(o *Options) { c.mut(fb, o) })
			got := 0
			for i := 0; i < 8; i++ {
				_, err := p.Provision(context.Background(), intentFor(fmt.Sprintf("TOK%d", i)))
				if errors.Is(err, burst.ErrQuota) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				got++
			}
			if got != c.slots {
				t.Fatalf("granted %d slots, want %d", got, c.slots)
			}
		})
	}
}

func TestSlotComesBackAfterTeardown(t *testing.T) {
	p := newProvider(t, newFake(), func(o *Options) { o.MaxSlots = 1 })
	ctx := context.Background()
	r, err := p.Provision(ctx, intentFor(tok1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Provision(ctx, intentFor(tok2)); !errors.Is(err, burst.ErrQuota) {
		t.Fatalf("second slot: %v", err)
	}
	if err := p.Teardown(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Provision(ctx, intentFor(tok2)); err != nil {
		t.Fatalf("slot not returned: %v", err)
	}
}

func TestConcurrentProvisionNeverOversubscribes(t *testing.T) {
	fb := newFake()
	fb.total = 3
	p := newProvider(t, fb, nil)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Provision(context.Background(), intentFor(fmt.Sprintf("TOK%d", i))); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 {
		t.Fatalf("%d of 24 concurrent provisions were granted, want 3", ok.Load())
	}
}

func TestTeardownFailureKeepsTheReservation(t *testing.T) {
	fb := newFake()
	p := newProvider(t, fb, func(o *Options) { o.MaxSlots = 1 })
	ctx := context.Background()
	r, _ := p.Provision(ctx, intentFor(tok1))
	fb.teardownErr = errors.New("daemon wedged")
	err := p.Teardown(ctx, r.ID)
	if CodeOf(err) != "E698" || !errors.Is(err, burst.ErrUnavailable) {
		t.Fatalf("want a retryable E698, got %v", err)
	}
	if got, _ := p.List(ctx, burst.OwnLabels(coord)); len(got) != 1 {
		t.Fatalf("a failed teardown dropped the record: %+v", got)
	}
	fb.teardownErr = nil
	if err := p.Teardown(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.List(ctx, burst.OwnLabels(coord)); len(got) != 0 {
		t.Fatalf("still listed: %+v", got)
	}
}

func TestTeardownIgnoresIdsItNeverIssued(t *testing.T) {
	fb := newFake()
	p := newProvider(t, fb, nil)
	for _, id := range []string{"", "x", "../etc", "nself-ci-1", "burst-", "burst-a b", "other-job-7"} {
		if err := p.Teardown(context.Background(), id); err != nil {
			t.Errorf("%q: %v", id, err)
		}
	}
	if len(fb.torn) != 0 {
		t.Fatalf("the adapter was asked to remove %v: only ids in the burst- namespace may reach it", fb.torn)
	}
}

func TestListMergesLeaksAndReservations(t *testing.T) {
	fb := newFake()
	p := newProvider(t, fb, nil)
	ctx := context.Background()
	if _, err := p.Provision(ctx, intentFor(tok1)); err != nil {
		t.Fatal(err)
	}
	old := epoch.Add(-time.Hour)
	// a container a killed coordinator left running, and one of another coordinator
	fb.add(Found{ID: "burst-" + tok2, CreatedAt: old, Labels: map[string]string{roleKey: roleValue, coordKey: coord, intentKey: tok2}})
	fb.add(Found{ID: "burst-OTHER", CreatedAt: old, Labels: map[string]string{roleKey: roleValue, coordKey: "someone-else"}})
	got, err := p.List(ctx, burst.OwnLabels(coord))
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %+v", err, got)
	}
	byID := map[string]burst.Resource{}
	for _, r := range got {
		byID[r.ID] = r
	}
	if !byID["burst-"+tok2].CreatedAt.Equal(old) || byID["burst-"+tok2].Labels[intentKey] != tok2 {
		t.Fatalf("leak lost its age or intent: %+v", byID["burst-"+tok2])
	}
	if err := p.Teardown(ctx, "burst-"+tok2); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.List(ctx, burst.OwnLabels(coord)); len(got) != 1 {
		t.Fatalf("the leak is still listed: %+v", got)
	}
}

func TestListReportsAnUnreadableRuntimeNeverEmpty(t *testing.T) {
	fb := newFake()
	fb.listErr = errors.New("cannot read")
	p := newProvider(t, fb, nil)
	got, err := p.List(context.Background(), nil)
	if err == nil || got != nil || !errors.Is(err, burst.ErrUnavailable) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestProvisionAdoptsASandboxTheRuntimeHolds(t *testing.T) {
	fb := newFake()
	fb.total = 1
	fb.add(Found{ID: "burst-" + tok1, CreatedAt: epoch.Add(-time.Minute), Labels: map[string]string{roleKey: roleValue, coordKey: coord, intentKey: tok1}})
	p := newProvider(t, fb, nil)
	r, err := p.Provision(context.Background(), intentFor(tok1))
	if err != nil {
		t.Fatalf("a restarted coordinator must adopt its own sandbox even when the host is full: %v", err)
	}
	if !r.CreatedAt.Equal(epoch.Add(-time.Minute)) || len(fb.prepared) != 0 {
		t.Fatalf("adopted %+v prepared %v", r, fb.prepared)
	}
}

func TestPrepareFailureReservesNothing(t *testing.T) {
	fb := newFake()
	fb.prepareErr = errors.New("adapter broke")
	p := newProvider(t, fb, nil)
	if _, err := p.Provision(context.Background(), intentFor(tok1)); CodeOf(err) != "E698" {
		t.Fatalf("%v", err)
	}
	if p.count() != 0 {
		t.Fatal("a failed prepare left a reservation")
	}
}

func TestCancelledContextIsNotWrapped(t *testing.T) {
	fb := newFake()
	fb.prepareErr = context.Canceled
	p := newProvider(t, fb, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Provision(ctx, intentFor(tok1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestEstimateUsesMeasuredLatencyPerLevel(t *testing.T) {
	fb := newFake()
	p := newProvider(t, fb, nil)
	ctx := context.Background()
	spec := burst.Spec{Platform: "linux/amd64"}
	e, _ := p.Estimate(ctx, spec)
	if e.Class != burst.ClassLocal || e.LatencyMs != priors[sandboxLevel] {
		t.Fatalf("before any probe the slowest level of the platform is assumed: %+v", e)
	}
	r, _ := p.Provision(ctx, intentFor(tok1)) // the probe offered the container level
	if e, _ = p.Estimate(ctx, spec); e.LatencyMs != priors[containerLevel] {
		t.Fatalf("container prior expected, got %+v", e)
	}
	p.Observe(r.ID, 1000*time.Millisecond)
	if e, _ = p.Estimate(ctx, spec); e.LatencyMs != 1000 {
		t.Fatalf("first measurement replaces the prior: %+v", e)
	}
	p.Observe(r.ID, 2000*time.Millisecond)
	if e, _ = p.Estimate(ctx, spec); e.LatencyMs != 1250 {
		t.Fatalf("smoothed 1/4: %+v", e)
	}
	p.Observe(r.ID, 0)
	p.Observe("burst-unknown", time.Second)
	if e, _ = p.Estimate(ctx, spec); e.LatencyMs != 1250 {
		t.Fatalf("zero and unknown ids are ignored: %+v", e)
	}
	fb.setDown(true)
	if e, err := p.Estimate(ctx, spec); err != nil || e.LatencyMs != 1250 {
		t.Fatalf("Estimate must not call the runtime: %+v %v", e, err)
	}
}

func TestVMLevelPriorOnApple(t *testing.T) {
	fb := newFake()
	fb.platform, fb.level = "darwin/arm64", vmLevel
	p := newProvider(t, fb, nil)
	if e, _ := p.Estimate(context.Background(), burst.Spec{}); e.LatencyMs != priors[vmLevel] {
		t.Fatalf("%+v", e)
	}
	in := intentFor(tok1)
	in.Spec.Platform = "darwin/arm64"
	r, err := p.Provision(context.Background(), in)
	if err != nil || p.Level(r.ID) != vmLevel {
		t.Fatalf("level %q", p.Level(r.ID))
	}
}

func TestNewNeedsABackend(t *testing.T) {
	if _, err := New(Options{}); CodeOf(err) != "E698" {
		t.Fatal(err)
	}
}

func TestCleanupHelperIsNotASecondSlot(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	found := []Found{{ID: "burst-X1"}, {ID: "burst-X1-clean"}, {ID: "burst-Y2"}}
	if got := p.used(Offer{}, found); got != 2 {
		t.Fatalf("used %d slots, want 2 (the helper of burst-X1 is not a slot)", got)
	}
}

func TestProvisionGivesUpWithItsContextWhileAnotherHoldsTheLock(t *testing.T) {
	p := newProvider(t, newFake(), nil)
	p.provSem <- struct{}{} // another Provision is stuck in a runtime call
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.Provision(ctx, intentFor("WAIT1")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a waiting Provision must return its ctx error, got %v", err)
	}
}
