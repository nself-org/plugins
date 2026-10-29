package watch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func only(t *testing.T, rep PassReport) Decision {
	t.Helper()
	if len(rep.Decisions) != 1 {
		t.Fatalf("decisions %+v", rep.Decisions)
	}
	return rep.Decisions[0]
}

func TestCandidateRules(t *testing.T) {
	newer, older := t0.Add(-time.Hour), t0.Add(-2*time.Hour)
	heads := []Head{
		{SHA: sha(1), Branch: "feat/x", Extra: true, CommittedAt: older},
		{SHA: sha(2), PR: 7},
		{SHA: sha(3), Branch: "release", Protected: true},
		{SHA: sha(4), Branch: "feat/y", Extra: true, CommittedAt: newer},
		{SHA: sha(5), Branch: "main", Default: true, Protected: true},
	}
	Order(heads)
	var got []string
	for _, h := range heads {
		got = append(got, h.SHA[39:])
	}
	if strings.Join(got, "") != "53241" {
		t.Fatalf("order %v: want default, protected, pull request, then extra newest first", got)
	}

	r := newRig(t, ownerHead("o/r", 1), ownerHead("o/r", 2), Head{Repo: "o/r", SHA: sha(3), PR: 3, Trust: model.TrustClass("untrusted")})
	rep := r.w.Pass(context.Background())
	if rep.Ran != 1 || r.runner.count() != 1 || r.runner.jobs[0].Head.SHA != sha(1) {
		t.Fatalf("one commit per pass, the first in order: %+v", rep.Decisions)
	}
	if rep.Decisions[1].Reason != SkipBusy || rep.Decisions[2].Reason != SkipApproval {
		t.Fatalf("decisions %+v", rep.Decisions)
	}
	if r.runner.jobs[0].Context != report.Context || len(r.runner.jobs[0].Only) != 0 {
		t.Fatalf("job %+v", r.runner.jobs[0])
	}
	if got := r.gh.states(sha(1), report.Context); len(got) != 1 || got[0] != "success" {
		t.Fatalf("result %v", got)
	}
	rep = r.w.Pass(context.Background())
	if rep.Decisions[0].Reason != SkipFinal || rep.Decisions[1].Action != "run" {
		t.Fatalf("second pass %+v", rep.Decisions)
	}
}

func TestForkNeverRuns(t *testing.T) {
	r := newRig(t, Head{Repo: "o/r", SHA: sha(1), PR: 4, Fork: true, Trust: model.TrustClass("owner")})
	r.w.P.Isolated = func(context.Context) bool { return true }
	for i := 0; i < 3; i++ {
		if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipFork {
			t.Fatalf("decision %+v", d)
		}
	}
	if r.runner.count() != 0 || r.gh.claims != 0 || len(r.gh.posts) != 0 {
		t.Fatal("a fork head must never be claimed, run or reported")
	}
}

func TestCollaboratorNeedsIsolatedRunner(t *testing.T) {
	h := Head{Repo: "o/r", SHA: sha(1), PR: 2, Trust: model.TrustClass("collaborator")}
	r := newRig(t, h)
	d := only(t, r.w.Pass(context.Background()))
	if d.Reason != SkipIsolated || d.Code != CodeIsolated || r.runner.count() != 0 || r.gh.claims != 0 {
		t.Fatalf("no isolated runner: %+v, runs %d", d, r.runner.count())
	}
	r.w.P.Isolated = func(context.Context) bool { return true }
	if d = only(t, r.w.Pass(context.Background())); d.Action != "run" || !r.runner.jobs[0].Isolated {
		t.Fatalf("isolated runner present: %+v", d)
	}
}

func TestTeamOwnedSkipped(t *testing.T) {
	r := newRig(t, ownerHead("o/r", 1))
	r.w.P.TeamOwned = func(_ context.Context, repo string) bool { return repo == "o/r" }
	if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipTeam {
		t.Fatalf("decision %+v", d)
	}
	if r.runner.count() != 0 || r.gh.claims != 0 || len(r.gh.posts) != 0 {
		t.Fatal("a personal watcher must not post for a team-owned project")
	}
}

func TestNotReadyNoClaim(t *testing.T) {
	r := newRig(t, ownerHead("o/r", 1))
	var docs []demand.Document
	r.w.P.Ready = func(context.Context) (bool, []string) { return false, []string{"load high"} }
	r.w.P.Publish = func(d demand.Document) error { docs = append(docs, d); return nil }
	if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipNotReady {
		t.Fatalf("decision %+v", d)
	}
	if r.gh.claims != 0 || r.runner.count() != 0 {
		t.Fatal("a not-ready host claims nothing")
	}
	if len(docs) != 1 || docs[0].Ready || docs[0].Queued != 1 || !docs[0].Watcher.Alive || docs[0].ReadyWhy[0] != "load high" {
		t.Fatalf("demand %+v", docs)
	}
}

func TestStatusContextPinned(t *testing.T) {
	r := newRig(t, ownerHead("o/r", 1))
	r.w.P.ResultContext = func(context.Context, string, string, string) string { return report.WatchContext }
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(1), report.WatchContext); len(got) != 1 || got[0] != "success" {
		t.Fatalf("advisory result %v", got)
	}
	if got := r.gh.states(sha(1), report.Context); len(got) != 0 {
		t.Fatalf("a pinned repository never gets nself-ci: %v", got)
	}
	r = newRig(t, ownerHead("o/r", 2))
	r.w.P.ResultContext = nil // protection unreadable: treated as pinned
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(2), report.WatchContext); len(got) != 1 {
		t.Fatalf("unreadable protection: %v", got)
	}
	r = newRig(t, ownerHead("o/r", 3))
	r.w.P.ResultContext = func(context.Context, string, string, string) string { return report.WatchContext }
	r.w.C.StatusContext = report.Context // watch.status_context overrides
	r.w.Pass(context.Background())
	if got := r.gh.states(sha(3), report.Context); len(got) != 1 {
		t.Fatalf("override: %v", got)
	}
}

func TestRetryErrorNotFailure(t *testing.T) {
	r := newRig(t, ownerHead("o/r", 1))
	r.runner.state = func(int, Job) report.State { return report.Error }
	for i := 0; i < 12; i++ { // two passes per retry gap, a little over an hour
		r.w.Pass(context.Background())
		r.gh.advance(5*time.Minute + time.Second)
	}
	if got := r.gh.states(sha(1), report.Context); len(got) != 4 || r.runner.count() != 4 {
		t.Fatalf("an error is retried 3 times: results %v, runs %d", got, r.runner.count())
	}
	if r.gh.claims != 4 {
		t.Fatalf("each retry posts a new claim: %d", r.gh.claims)
	}
	if d := only(t, r.w.Pass(context.Background())); d.Reason != SkipFinal {
		t.Fatalf("after the budget the error stays final: %+v", d)
	}
	gaps := []time.Time{}
	for _, s := range r.gh.st[sha(1)] {
		if s.Context == report.Context {
			gaps = append(gaps, s.CreatedAt)
		}
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i].Sub(gaps[i-1]) < DefaultRetryGap {
			t.Fatalf("retries %v apart, want at least %v", gaps[i].Sub(gaps[i-1]), DefaultRetryGap)
		}
	}

	r = newRig(t, ownerHead("o/r", 2))
	r.runner.state = func(int, Job) report.State { return report.Failure }
	for i := 0; i < 6; i++ {
		r.w.Pass(context.Background())
		r.gh.advance(11 * time.Minute)
	}
	if r.runner.count() != 1 {
		t.Fatalf("a failure is never retried: %d runs", r.runner.count())
	}
}

func TestQuotaStopsPass(t *testing.T) {
	r := newRig(t, ownerHead("o/r", 1))
	r.scan.err = &QuotaError{RetryAt: t0.Add(10 * time.Minute)}
	rep := r.w.Pass(context.Background())
	if !rep.QuotaUntil.Equal(t0.Add(10*time.Minute)) || len(rep.Decisions) != 0 || r.scan.calls != 1 {
		t.Fatalf("quota pass %+v", rep)
	}
	r.scan.err = nil
	r.gh.advance(5 * time.Minute)
	if rep = r.w.Pass(context.Background()); r.scan.calls != 1 || len(rep.Decisions) != 0 {
		t.Fatal("no scan before the recorded reset")
	}
	if n := r.w.Next(r.gh.clock()); n < 5*time.Minute {
		t.Fatalf("the next pass waits for the reset: %v", n)
	}
	r.gh.advance(6 * time.Minute)
	if rep = r.w.Pass(context.Background()); r.scan.calls != 2 || rep.Ran != 1 {
		t.Fatalf("after the reset %+v", rep)
	}
}
