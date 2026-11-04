package main

// Purpose: prove the hosted lease wiring of the coordinator start (D-1297): serveV15 registers the hosted lease
//          profiles on its lease engine and runs the hosted poll loop, so a job placed on a fake hosted runner
//          outlives its initial lease TTL on the loop's renewals alone, and the loop watches it to completion
//          without racing the carrier for the result.
// Inputs:  the production TEAM entrypoint, a fake hosted provider registered once for the process, the real
//          world builder, dispatcher, transport registry and hosted driver on the coordinator's store file.
// Outputs: pass or fail.
// Constraints: no network and no real provider: the fake answers every call; the fake runner is "included"
//          (no paid placement) and the economics defaults are untouched; the provider describes a runner only
//          while this test runs, so other tests' worlds stay as they were.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// pollRunFor is how long the fake hosted run stays running: longer than the initial lease TTL (pollLeaseTTL),
// so only the poll loop's renewals keep the lease alive.
const (
	pollRunFor   = 3 * time.Second
	pollLeaseTTL = time.Second
)

func TestServeHostedPollLoopCarriesJobToCompletion(t *testing.T) {
	pollProvOnce.Do(func() {
		pollProv.Fake = fake.New(fake.Options{Evidence: pollEvidence})
		if err := providers.Register(pollProv); err != nil {
			t.Fatal(err)
		}
		if err := runtime.RegisterCredential(pollProv.ID(), func(context.Context) rest.AuthFunc {
			return func(context.Context) (string, error) { return "synthetic-test-token", nil }
		}); err != nil {
			t.Fatal(err)
		}
	})
	pollProv.mu.Lock()
	pollProv.Fake, pollProv.on, pollProv.dispatches = fake.New(fake.Options{Evidence: pollEvidence}), true, 0
	pollProv.mu.Unlock()
	t.Cleanup(func() { pollProv.mu.Lock(); pollProv.on = false; pollProv.mu.Unlock() })
	runtime.DefaultContributor().ForgetCredentials()
	runtime.Default().SetFacts(func(context.Context, model.Capability, protocol.Lease) (runtime.AttemptFacts, error) {
		return runtime.AttemptFacts{N: 1, Project: "project", Repo: "o/r", TimeoutMs: 300000}, nil
	})
	t.Cleanup(func() { runtime.Default().SetFacts(nil) })
	obs := &pollObserved{ready: make(chan struct{}), polls: map[string][]lease.LiveState{}}
	oldEvery, oldObserve := hostedPollEvery, hostedPollObserve
	hostedPollEvery, hostedPollObserve = 20*time.Millisecond, obs.observe // read once, when serve starts
	t.Cleanup(func() { hostedPollEvery, hostedPollObserve = oldEvery, oldObserve })

	st, _ := startGitHubServe(t, nil, "owner/repo", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture offline", http.StatusForbidden)
	}))
	// the coordinator describes hosted runners for GitHub repositories only; this test's world names the fake
	runtime.DefaultContributor().SetTargetsFor(func(_ context.Context, provider string) ([]providers.Query, error) {
		if provider != pollProv.ID() {
			return nil, nil
		}
		return []providers.Query{{Project: "project"}}, nil
	})
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "pipe-poll", Revision: githubServeRev, Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-poll", PipelineID: "pipe-poll", Name: "poll", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: "att-poll", JobID: "job-poll", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	sink := &pollSink{eng: lease.New(st, nil, lease.Config{})}
	buildWorld := func(ctx context.Context, pl sched.Pipeline) (sched.World, string, error) {
		w, digest, err := world.Build(ctx, world.Deps{Store: st, Policy: func(context.Context, string) (sched.Policy, error) {
			return sched.Policy{Preset: "balanced", Trust: serveHostedTrust(t)}, nil
		}}, pl)
		w.Costs = map[string]sched.Cost{"fake-hosted-1": {Class: "included", RateMicro: 1, BillingIncrementMs: 60000}}
		w.Allowances = map[string]sched.Allowance{"fake": {RemainingEstimated: 1_000_000, Known: true, CeilingKnown: true}}
		return w, digest, err
	}
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", World: buildWorld, Transport: pollCarrier{sink}, Poll: time.Millisecond, LeaseTTL: pollLeaseTTL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	job := sched.Job{Key: "att-poll", ID: "att-poll", Name: "poll", Pipeline: sched.Pipeline{ID: "pipe-poll", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true, Project: "project"}
	start := time.Now()
	a, err := d.Submit(ctx, dispatch.Job{AttemptID: "att-poll", Sched: job, Revision: githubServeRev, Model: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "5m"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.Local || a.RunnerID != "fake-hosted-1" || a.Epoch == 0 {
		t.Fatalf("the job must be admitted on the fake hosted runner: %+v", a)
	}
	select {
	case <-obs.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("the coordinator's hosted poll loop never polled the admitted lease")
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if res, _ := sink.got(); len(res) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	res, sinkErrs := sink.got()
	if len(res) != 1 || res[0].LeaseID != a.LeaseID || len(sinkErrs) != 0 {
		t.Fatalf("one accepted result for the admitted lease expected: results=%+v errors=%v", res, sinkErrs)
	}
	if took := time.Since(start); took < pollRunFor {
		t.Fatalf("the run finished after %v, before the fake's %v: the lease TTL was never at stake", took, pollRunFor)
	}
	if state, _, err := st.AttemptStateEpoch(ctx, "att-poll"); err != nil || state != "finalizing" {
		t.Fatalf("attempt after the carrier's result = %q (%v), want finalizing: the lease outlived its %v TTL only through the poll loop", state, err, pollLeaseTTL)
	}
	time.Sleep(200 * time.Millisecond) // a poll that was in flight when the result landed has finished
	polls, pollErrs := obs.of(a.LeaseID)
	running := 0
	for _, s := range polls {
		switch s {
		case lease.LiveRunning:
			running++
		case lease.LiveFinished:
			t.Fatalf("the poll loop finished a lease its carrier still held: %v", polls)
		}
	}
	if running < 3 || len(pollErrs) != 0 {
		t.Fatalf("the poll loop must renew the running lease at the profile interval: running polls=%d errors=%v", running, pollErrs)
	}
	time.Sleep(500 * time.Millisecond) // five profile intervals: a completed lease is no longer polled
	if after, _ := obs.of(a.LeaseID); len(after) != len(polls) {
		t.Fatalf("the poll loop kept polling a completed lease: %d polls, then %d", len(polls), len(after))
	}
	pollProv.mu.Lock()
	n := pollProv.dispatches
	pollProv.mu.Unlock()
	if n != 1 {
		t.Fatalf("the fake run was dispatched %d times, want once", n)
	}
}
