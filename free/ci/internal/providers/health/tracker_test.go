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
	"github.com/nself-org/plugins/free/ci/internal/store"
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

// TestNoRetryOnAuth proves acceptance line 3 through the real hosted stack (the GitHub-hosted
// provider against the fake API, placed by the scheduler, delivered by the dispatcher). A 401 on
// dispatch ends the carrier with a provider error of class auth, code E701, after one request, and
// never counts against the provider's health; the run loop does not retry that class
// (internal/run TestRemoteAwaitProviderRefusalIsNotRetried). A 500 ends it with class transient,
// E704, which the run loop retries within retry.infra_max (TestRemoteAwaitTransientProviderErrorIsRetried),
// and the store refuses the retry past that bound.
func TestNoRetryOnAuth(t *testing.T) {
	clk := newClock()
	tr := health.New(health.Options{Clock: clk.Now})
	api := newFakeAPI(t)
	s := newStack(t, api, tr)
	api.setMode(modeUnauth)
	aid := s.queue("auth", 1)
	if _, err := s.submit(context.Background(), aid, "auth", true); err != nil {
		t.Fatal(err)
	}
	var pe *providers.Error
	if err := s.waitDeliveryErrors(1)[0]; !errors.As(err, &pe) || pe.Class != providers.Auth || pe.Code != "E701" {
		t.Fatalf("a 401 on dispatch must end the carrier with E701 (auth): %v", err)
	}
	before := api.total()
	time.Sleep(200 * time.Millisecond)
	if after := api.total(); after != before {
		t.Fatalf("the provider kept sending after the refusal: %d -> %d requests", before, after)
	}
	if h := tr.HealthOf(context.Background(), provID); h.State != "up" {
		t.Fatalf("a credential refusal is not an outage: %+v", h)
	}
	if n := api.count("POST /repos/o/r/actions/workflows/"); n > 1 {
		t.Fatalf("workflow dispatch was sent %d times after a 401", n)
	}

	api.setMode(mode5xx)
	aid2 := s.queue("flaky", 1)
	if _, err := s.submit(context.Background(), aid2, "flaky", true); err != nil {
		t.Fatal(err)
	}
	if err := s.waitDeliveryErrors(2)[1]; !errors.As(err, &pe) || pe.Class != providers.Transient || pe.Code != "E704" {
		t.Fatalf("a 500 on dispatch must end the carrier with E704 (transient): %v", err)
	}
	// infra_max is 1: the lost first attempt may be retried, the lost second one may not.
	ctx := context.Background()
	_, epoch, _ := s.st.AttemptStateEpoch(ctx, aid2)
	if err := s.st.Transition(ctx, aid2, "lost", "retryable", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
		t.Fatalf("the first retry must be authorized: %v", err)
	}
	_, epoch, _ = s.st.AttemptStateEpoch(ctx, aid2)
	if err := s.st.Transition(ctx, aid2, "retryable", "queued", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
		t.Fatal(err)
	}
	api.setMode(modeUnauth) // the 500s above may have opened the breaker; a refusal loses the attempt without it
	tr.Breaker(provID).Observe(outcome("", 200), clk.Now())
	aid3 := s.queue("flaky", 2)
	if _, err := s.submit(ctx, aid3, "flaky", true); err != nil {
		t.Fatal(err)
	}
	s.waitDeliveryErrors(3)
	_, epoch, _ = s.st.AttemptStateEpoch(ctx, aid3)
	if err := s.st.Transition(ctx, aid3, "lost", "retryable", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err == nil {
		t.Fatal("a second retry past infra_max must be refused")
	}
}

// TestFirstOutcomeAfterRestartKeepsSavedState: the first outcome a restarted process sees (here a
// failure during the probe window, before anything read the health) starts from the saved state, so
// the saved outage is not forgotten and the probe gap doubles from where it was.
func TestFirstOutcomeAfterRestartKeepsSavedState(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	st := openStore(t, clk.Now)
	a := health.New(health.Options{Clock: clk.Now, Store: st})
	failN(a, health.DownAfter)
	clk.advance(health.ProbeAfter + time.Second)
	b := health.New(health.Options{Clock: clk.Now, Store: st})
	b.Observe(outcome("transient", 503)) // no HealthOf, no Load before it
	got := b.HealthOf(ctx, "github-hosted")
	if got.State != "down" || !got.RetryAt.After(clk.Now().Add(health.ProbeAfter)) {
		t.Fatalf("a failed probe after a restart must double the saved gap: %+v", got)
	}
	c := health.New(health.Options{Clock: clk.Now, Store: st})
	c.Observe(outcome("", 200)) // the first outcome is an answer: the saved outage ends and is saved
	clk.advance(2 * time.Second)
	d := health.New(health.Options{Clock: clk.Now, Store: st})
	if got = d.HealthOf(ctx, "github-hosted"); got.State != "up" {
		t.Fatalf("the recovery seen first after a restart must be saved: %+v", got)
	}
}
