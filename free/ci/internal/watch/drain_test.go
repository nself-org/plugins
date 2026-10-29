package watch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// drainRig is a rig whose runner blocks and whose After hands each timer to the test.
func drainRig(t *testing.T) (*rig, chan chan time.Time) {
	r := newRig(t, ownerHead("o/r", 1))
	r.runner.block, r.runner.started = make(chan struct{}), make(chan struct{}, 1)
	timers := make(chan chan time.Time, 4)
	r.w.P.After = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		timers <- ch
		return ch
	}
	return r, timers
}

func TestDrainOnSigterm(t *testing.T) {
	// the drain ends: the attempt is cancelled and closed with an error, the claim is not orphaned
	r, timers := drainRig(t)
	ctx, sigterm := context.WithCancel(context.Background())
	done := make(chan PassReport, 1)
	go func() { done <- r.w.Pass(ctx) }()
	<-r.runner.started
	sigterm()
	timer := <-timers // the drain started; the attempt still runs
	select {
	case <-done:
		t.Fatal("the pass ended before the drain")
	default:
	}
	timer <- t0
	rep := <-done
	if got := r.gh.states(sha(1), report.Context); len(got) != 1 || got[0] != "error" {
		t.Fatalf("cancelled attempt result %v (%+v)", got, rep.Decisions)
	}
	if p := r.gh.posts[0]; !strings.Contains(p.Description, "cancelled") {
		t.Fatalf("description %q", p.Description)
	}

	// the attempt finishes inside the drain: its own result is posted
	r, timers = drainRig(t)
	ctx, sigterm = context.WithCancel(context.Background())
	go func() { done <- r.w.Pass(ctx) }()
	<-r.runner.started
	sigterm()
	<-timers
	close(r.runner.block)
	<-done
	if got := r.gh.states(sha(1), report.Context); len(got) != 1 || got[0] != "success" {
		t.Fatalf("finished attempt result %v", got)
	}

	// a loop cancelled between passes stops without another pass
	r = newRig(t, ownerHead("o/r", 1))
	ctx, sigterm = context.WithCancel(context.Background())
	passes := 0
	r.w.P.After = func(time.Duration) <-chan time.Time { sigterm(); return make(chan time.Time) }
	r.w.Run(ctx, false, func(PassReport) { passes++ })
	if passes != 0 || r.runner.count() != 0 {
		t.Fatalf("passes %d after SIGTERM", passes)
	}
}

type fixedHosted struct{ r hostedtier.Result }

func (f fixedHosted) Decide(context.Context, string, string, time.Time) hostedtier.Result { return f.r }

func TestResidualAggregation(t *testing.T) {
	use := hostedtier.Result{Decision: hostedtier.Use, Reason: hostedtier.ReasonSuccess, Residual: []string{"secrets.gitleaks"}}
	failRes := func(int, Job) report.State { return report.Failure }

	// unpinned: nself-ci is the aggregate, failure when the residual failed
	r := newRig(t, ownerHead("o/r", 1))
	r.w.P.Hosted, r.runner.state = fixedHosted{use}, failRes
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(1), report.Context); len(got) != 1 || got[0] != "failure" {
		t.Fatalf("unpinned aggregate %v", got)
	}
	if j := r.runner.jobs[0]; len(j.Only) != 1 || j.Only[0] != "secrets.gitleaks" {
		t.Fatalf("only the residual checks run: %+v", j)
	}

	// pinned: nself-ci/residual plus a failure alert; nothing else is written
	r = newRig(t, ownerHead("o/r", 2))
	r.w.P.Hosted, r.runner.state = fixedHosted{use}, failRes
	r.w.P.ResultContext = func(context.Context, string, string, string) string { return report.WatchContext }
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(2), ResidualContext); len(got) != 1 || got[0] != "failure" || len(r.alerts) != 1 {
		t.Fatalf("pinned residual %v, alerts %v", got, r.alerts)
	}
	if len(r.gh.states(sha(2), report.Context))+len(r.gh.states(sha(2), report.WatchContext)) != 0 {
		t.Fatal("the pinned result contexts stay untouched")
	}
	if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipFinal {
		t.Fatalf("the residual result is final: %+v", d)
	}

	// a red hosted result stays red even when the residual passed
	red := use
	red.Reason = hostedtier.ReasonFailure
	r = newRig(t, ownerHead("o/r", 3))
	r.w.P.Hosted = fixedHosted{red}
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(3), report.Context); len(got) != 1 || got[0] != "failure" {
		t.Fatalf("red hosted %v", got)
	}

	// wait: nothing is claimed while the hosted run is inside its timers
	r = newRig(t, ownerHead("o/r", 4))
	r.w.P.Hosted = fixedHosted{hostedtier.Result{Decision: hostedtier.Wait}}
	if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipHosted || r.gh.claims != 0 {
		t.Fatalf("hosted wait %+v", d)
	}
}
