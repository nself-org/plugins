package auto

// Purpose: A4 the hosted tier on fixtures: a green hosted result stands, a red one stays red, a
// queue that outlasts its timer hands over, and a billing-blocked account hands over then stays quiet.
// Inputs: hostedtier.Observer over a fake read-only funnel. Outputs: pass or fail.
// Constraints: no network; the clock is injected.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
)

var at = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type funnel struct {
	runs  []hostedtier.CheckRun
	notes map[int64][]string
	calls int
}

func (f *funnel) CheckRuns(context.Context, string) ([]hostedtier.CheckRun, error) {
	f.calls++
	return f.runs, nil
}

func (f *funnel) Annotations(_ context.Context, id int64) ([]string, error) {
	f.calls++
	return f.notes[id], nil
}

func hostedRun(status, conclusion string, created time.Time) hostedtier.CheckRun {
	return hostedtier.CheckRun{ID: 7, Name: hostedtier.DefaultContext, AppID: hostedtier.GitHubActionsAppID, Status: status, Conclusion: conclusion, CreatedAt: created, StartedAt: created}
}

func observe(f *funnel) *hostedtier.Observer {
	tr := health.New(health.Options{Clock: func() time.Time { return at }})
	return &hostedtier.Observer{Funnel: f, Breaker: hostedtier.NewHealthBreaker(tr, "github-hosted"), Now: func() time.Time { return at }}
}

func TestConformanceA4Hosted(t *testing.T) {
	cases := []struct {
		name string
		run  hostedtier.CheckRun
		want hostedtier.Decision
		why  string
	}{
		{"green", hostedRun("completed", "success", at), hostedtier.Use, hostedtier.ReasonSuccess},
		{"red", hostedRun("completed", "failure", at), hostedtier.Use, hostedtier.ReasonFailure},
		{"young queue", hostedRun("queued", "", at.Add(-10*time.Second)), hostedtier.Wait, hostedtier.ReasonQueued},
		{"stale queue", hostedRun("queued", "", at.Add(-time.Minute)), hostedtier.HandOver, hostedtier.ReasonQueuedLong},
	}
	for _, c := range cases {
		got := observe(&funnel{runs: []hostedtier.CheckRun{c.run}}).Decide(context.Background(), "sha", at)
		if got.Decision != c.want || got.Reason != c.why {
			t.Fatalf("%s: got %s/%s, want %s/%s", c.name, got.Decision, got.Reason, c.want, c.why)
		}
	}
	// billing blocked: the first call hands over, the breaker window then makes no request at all
	f := &funnel{runs: []hostedtier.CheckRun{hostedRun("completed", "failure", at)}, notes: map[int64][]string{7: {"recent account payments have failed or your spending limit needs to be increased"}}}
	o := observe(f)
	first := o.Decide(context.Background(), "sha", at)
	if first.Decision != hostedtier.HandOver || first.Reason != hostedtier.ReasonBilling {
		t.Fatalf("billing: got %s/%s", first.Decision, first.Reason)
	}
	calls := f.calls
	again := o.Decide(context.Background(), "sha", at)
	if again.Reason != hostedtier.ReasonUnavailable || again.Requests != 0 || f.calls != calls {
		t.Fatalf("breaker window made requests: %+v", again)
	}
}
