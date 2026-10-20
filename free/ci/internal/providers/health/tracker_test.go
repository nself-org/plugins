package health_test

// Purpose: persistence, quota handling, drain-on-recovery and the no-retry rule of the health
//          tracker (P7-HOST-11).
// Inputs:  a real store, the fake API, an injected clock.
// Outputs: pass or fail.
// Constraints: no real time, no live network.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
)

func failN(tr *health.Tracker, n int) {
	for i := 0; i < n; i++ {
		tr.Observe(outcome("transient", 503))
	}
}

// TestHealthPersists proves acceptance line 4 (state survives a process restart through
// host-provider): a tracker that opened the breaker saved it; a second tracker on the same store,
// which has observed nothing (a restart, or `ci providers show` in another process), reads down with
// the same probe time, then reads the recovery the first one saved.
func TestHealthPersists(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	st := openStore(t, clk.Now)
	a := health.New(health.Options{Clock: clk.Now, Store: st})
	failN(a, health.DownAfter)
	if a.SaveErrors() != 0 {
		t.Fatalf("save errors: %d", a.SaveErrors())
	}
	want := a.HealthOf(ctx, "github-hosted")
	if want.State != "down" {
		t.Fatalf("first tracker: %+v", want)
	}
	b := health.New(health.Options{Clock: clk.Now, Store: st})
	if got := b.HealthOf(ctx, "github-jit"); got.State != "down" || !got.RetryAt.Equal(want.RetryAt) || !got.Since.Equal(want.Since) {
		t.Fatalf("a restarted tracker reads %+v, want %+v (one API family covers github-hosted and github-jit)", got, want)
	}
	if got := b.HealthOf(ctx, "gitlab-pipeline"); got.State != "up" {
		t.Fatalf("another API family must be unaffected: %+v", got)
	}
	clk.advance(health.ProbeAfter + time.Second)
	if got := b.HealthOf(ctx, "github-hosted"); got.State != "degraded" || got.Reason != health.ReasonProbing {
		t.Fatalf("after the probe gap the restarted tracker hands out the probe slot: %+v", got)
	}
	b.Observe(outcome("", 200))
	if got := b.HealthOf(ctx, "github-hosted"); got.State != "up" {
		t.Fatalf("one answer closes the breaker: %+v", got)
	}
	clk.advance(2 * time.Second) // the reload interval
	c := health.New(health.Options{Clock: clk.Now, Store: st})
	if got := c.HealthOf(ctx, "github-hosted"); got.State != "up" {
		t.Fatalf("the recovery is persisted too: %+v", got)
	}
	// A process that already observed an outcome keeps its own state over an older saved one.
	d := health.New(health.Options{Clock: clk.Now, Store: st})
	failN(d, health.DownAfter)
	d.SetStore(st)
	if got := d.HealthOf(ctx, "github-hosted"); got.State != "down" {
		t.Fatalf("local observations win over the stored fact: %+v", got)
	}
}

// TestDegradedOnQuota proves acceptance line 2: a 429 with Retry-After makes the provider degraded
// until RetryAt, placement still sees it available (State is not "down", which is the only state the
// world contributor turns into an offline runner), and the breaker does not open however many
// quota answers arrive. The outcomes come from a real REST funnel client against the fake API.
func TestDegradedOnQuota(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	tr := health.New(health.Options{Clock: clk.Now})
	install(t, tr)
	api := newFakeAPI(t)
	api.setMode(modeQuota)
	c := api.client()
	for i := 0; i < 12; i++ {
		_, err := c.Do(ctx, "GET", "/user", nil, nil, nil)
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Class != providers.Quota || pe.Code != "E703" {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	h := providers.HealthOf(ctx, "github-hosted")
	if h.State != "degraded" || h.Reason != health.ReasonQuota || !h.RetryAt.After(clk.Now()) {
		t.Fatalf("health during the window = %+v", h)
	}
	if tr.Breaker("github-hosted").Snapshot().Down {
		t.Fatal("quota answers must not open the breaker")
	}
	clk.advance(80 * time.Second)
	if h = providers.HealthOf(ctx, "github-hosted"); h.State != "degraded" {
		t.Fatalf("health before RetryAt = %+v", h)
	}
	clk.advance(20 * time.Second)
	if h = providers.HealthOf(ctx, "github-hosted"); h.State != "up" {
		t.Fatalf("health after RetryAt = %+v", h)
	}
}

// TestDrainOnRecovery: report.Drain runs once per down -> up move and not for other answers.
func TestDrainOnRecovery(t *testing.T) {
	var drains atomic.Int64
	clk := newClock()
	tr := health.New(health.Options{Clock: clk.Now, Drain: func(context.Context) (int, error) { drains.Add(1); return 0, nil }})
	tr.Observe(outcome("", 200))
	failN(tr, health.DownAfter-1)
	tr.Observe(outcome("", 200))
	tr.Wait()
	if drains.Load() != 0 {
		t.Fatalf("drain ran without an outage: %d", drains.Load())
	}
	failN(tr, health.DownAfter)
	tr.Observe(outcome("transient", 503)) // in flight while down
	tr.Observe(outcome("", 200))
	tr.Observe(outcome("", 200))
	tr.Wait()
	if drains.Load() != 1 || tr.Drains() != 1 {
		t.Fatalf("drains = %d, want exactly one for one recovery", drains.Load())
	}
}

// TestIgnoredProviders: a client that is not a provider API (the notify webhook) is not tracked.
func TestIgnoredProviders(t *testing.T) {
	tr := health.New(health.Options{Clock: newClock().Now, Ignore: []string{"notify"}})
	for i := 0; i < 10; i++ {
		tr.Observe(rest0("notify"))
		tr.Observe(rest0(""))
	}
	if h := tr.HealthOf(context.Background(), "notify"); h.State != "up" {
		t.Fatalf("notify tracked: %+v", h)
	}
}
