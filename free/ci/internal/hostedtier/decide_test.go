package hostedtier

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/health"
)

const billingText = "The job was not started because recent account payments have failed or your spending limit needs to be increased."

func TestHostedDecisionMatrix(t *testing.T) {
	s := time.Second
	cases := []struct {
		name   string
		runs   []CheckRun
		err    error
		pushed time.Time
		want   Decision
		reason string
	}{
		{"success", []CheckRun{run("completed", "success", t0, t0)}, nil, t0, Use, ReasonSuccess},
		{"failure", []CheckRun{run("completed", "failure", t0, t0)}, nil, t0, Use, ReasonFailure},
		{"queued 10s", []CheckRun{run("queued", "", t0.Add(-10*s), time.Time{})}, nil, t0, Wait, ReasonQueued},
		{"queued 40s", []CheckRun{run("queued", "", t0.Add(-40*s), time.Time{})}, nil, t0, HandOver, ReasonQueuedLong},
		{"running 2m", []CheckRun{run("in_progress", "", t0.Add(-3*time.Minute), t0.Add(-2*time.Minute))}, nil, t0, Wait, ReasonRunning},
		{"running 5m", []CheckRun{run("in_progress", "", t0.Add(-6*time.Minute), t0.Add(-5*time.Minute))}, nil, t0, HandOver, ReasonRunningLong},
		{"absent young", nil, nil, t0.Add(-10 * s), Wait, ReasonAbsent},
		{"absent old", nil, nil, t0.Add(-40 * s), HandOver, ReasonAbsentLong},
		{"cancelled", []CheckRun{run("completed", "cancelled", t0, t0)}, nil, t0, HandOver, ReasonCancelled},
		{"api error", nil, errBoom, t0.Add(-time.Hour), HandOver, ReasonAPIError},
		{"other app ignored", []CheckRun{{ID: 9, Name: DefaultContext, AppID: 1, Status: "completed", Conclusion: "success"}}, nil, t0.Add(-time.Hour), HandOver, ReasonAbsentLong},
		{"other name ignored", []CheckRun{{ID: 9, Name: "nself-ci/gates", AppID: GitHubActionsAppID, Status: "completed", Conclusion: "success"}}, nil, t0.Add(-time.Hour), HandOver, ReasonAbsentLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeFunnel{runs: c.runs, err: c.err}
			got := observer(t, f, Config{}, t0).Decide(context.Background(), "abc", c.pushed)
			if got.Decision != c.want || got.Reason != c.reason {
				t.Fatalf("got %s/%s, want %s/%s", got.Decision, got.Reason, c.want, c.reason)
			}
		})
	}
}

func TestBillingBlockedUnavailable(t *testing.T) {
	f := &fakeFunnel{runs: []CheckRun{run("completed", "failure", t0, t0)}, notes: map[int64][]string{7: {billingText}}}
	now := t0
	tr := health.New(health.Options{Clock: func() time.Time { return now }})
	o := &Observer{Funnel: f, Breaker: NewHealthBreaker(tr, "github-hosted"), Now: func() time.Time { return now }}
	got := o.Decide(context.Background(), "abc", t0)
	if got.Decision != HandOver || got.Reason != ReasonBilling {
		t.Fatalf("got %s/%s", got.Decision, got.Reason)
	}
	before := f.calls
	again := o.Decide(context.Background(), "abc", t0)
	if again.Reason != ReasonUnavailable || again.Requests != 0 || f.calls != before {
		t.Fatalf("breaker window must make no request: %+v calls %d->%d", again, before, f.calls)
	}
	now = t0.Add(time.Hour)
	if r := o.Decide(context.Background(), "abc", t0); r.Reason == ReasonUnavailable {
		t.Fatalf("a closed window must not stay unavailable: %+v", r)
	}
}

func TestRedHostedStaysRed(t *testing.T) {
	f := &fakeFunnel{runs: []CheckRun{run("completed", "failure", t0, t0)}, notes: map[int64][]string{7: {"exit status 1"}}}
	got := observer(t, f, Config{}, t0).Decide(context.Background(), "abc", t0)
	if got.Decision != Use || got.Reason != ReasonFailure {
		t.Fatalf("a code failure is used as red: %+v", got)
	}
	ok := observer(t, &fakeFunnel{runs: []CheckRun{run("completed", "success", t0, t0)}}, Config{}, t0).Decide(context.Background(), "abc", t0)
	if len(ok.Residual) != 1 || ok.Residual[0] != DefaultResidualID {
		t.Fatalf("residual: %v", ok.Residual)
	}
}

func TestHostedModeOffNoReads(t *testing.T) {
	f := &fakeFunnel{runs: []CheckRun{run("completed", "success", t0, t0)}}
	got := observer(t, f, Config{Mode: ModeOff}, t0).Decide(context.Background(), "abc", t0)
	if got.Decision != HandOver || got.Reason != ReasonOff || f.calls != 0 || got.Requests != 0 {
		t.Fatalf("off must not read: %+v calls %d", got, f.calls)
	}
}

func TestPaidNeedsAllowed(t *testing.T) {
	f := &fakeFunnel{runs: []CheckRun{run("completed", "success", t0, t0)}}
	got := observer(t, f, Config{Mode: ModePaid}, t0).Decide(context.Background(), "abc", t0)
	if got.Mode != ModeFree || len(got.Notes) != 1 || got.Notes[0] != NotePaidDenied {
		t.Fatalf("paid without PaidAllowed behaves as free and says so: %+v", got)
	}
	got = observer(t, f, Config{Mode: ModePaid, PaidAllowed: true}, t0).Decide(context.Background(), "abc", t0)
	if got.Mode != ModePaid || len(got.Notes) != 0 {
		t.Fatalf("paid with PaidAllowed: %+v", got)
	}
	if free := observer(t, f, Config{Mode: ModeFree, PaidAllowed: true}, t0).Decide(context.Background(), "abc", t0); free.Mode != ModeFree {
		t.Fatalf("free never selects paid: %+v", free)
	}
}
