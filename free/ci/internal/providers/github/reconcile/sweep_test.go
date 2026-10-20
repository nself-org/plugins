package reconcile

// Purpose: tests of the sweep Reconciler (P7-HOST-17 acceptance lines 1-4 and 7): restart recovery by head
// scan, redelivery once, the quota stop, the per-pass call bound, and poll-mode head scans.
// Inputs:  the fixture forge of rig_test.go.
// Outputs: pass or fail.
// Constraints: no live network; the clock is injected.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

func hasPath(sub string) func(reqRec) bool {
	return func(q reqRec) bool { return strings.Contains(q.Path, sub) }
}

// setTerminal makes the forge report a success for the head a delivery names (the test harness playing
// the pipeline that finished).
func (r *rig) setTerminal() {
	r.enq.hook = func(d coordinator.Delivery) {
		r.f.do(func() { r.f.repos[strings.ToLower(d.Project)].Status[d.Revision] = "success" })
	}
}

// TestReconcileAfterOutage proves acceptance line 1: the coordinator was down during 3 pushes and the
// token cannot list deliveries (403): after restart the sweep enqueues the 3 heads without a terminal
// status, each then gets one in the harness, and a head that already has success is not enqueued.
func TestReconcileAfterOutage(t *testing.T) {
	r := newRig(t, "acme/app")
	repo := r.f.repos["acme/app"]
	repo.HooksDenied = true
	r.policy(map[string]string{KeyIngress: `"webhook"`}) // webhook mode: hooks first, 403 falls back to the scan
	main, rel, dev, prHead, doneHead := sha('a', 1), sha('b', 2), sha('c', 3), sha('d', 4), sha('e', 5)
	repo.Branches = map[string]string{"main": main, "release/1": rel, "develop": dev}
	repo.Protected = []string{"release/1", "develop"}
	repo.Pulls = []fPull{{Number: 7, Head: prHead, Assoc: "MEMBER"}, {Number: 8, Head: doneHead}}
	repo.Status[doneHead] = "success" // built before the outage
	r.setTerminal()
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, d := range r.enq.deliveries() {
		got[d.Revision] = true
		if d.Source != "github" || !strings.HasPrefix(d.DeliveryID, "reconcile:acme/app:") || d.TriggerClass != "change" {
			t.Errorf("delivery %+v", d)
		}
	}
	if len(got) != 4 || !got[main] || !got[rel] || !got[dev] || !got[prHead] || got[doneHead] {
		t.Fatalf("enqueued %v, want main, release/1, develop and the PR head, not the built PR head", got)
	}
	for _, s := range []string{main, rel, dev, prHead} {
		if repo.Status[s] != "success" {
			t.Errorf("%s has no terminal status", s)
		}
	}
	f := r.enq.facts(t, indexOf(r.enq, prHead))
	if f.Kind != "pull_request" || f.Auth != "per-repo" || f.Ref != "refs/pull/7/head" || f.PRNumber != 7 || f.AuthorAssociation.Source != "api" || f.HeadCurrent.Source != "api" {
		t.Errorf("PR facts %+v", f)
	}
	pf := r.enq.facts(t, indexOf(r.enq, rel))
	if pf.Kind != "push" || pf.Ref != "refs/heads/release/1" || pf.ProtectedRef.Value == nil || !*pf.ProtectedRef.Value || pf.ProtectedRef.Source != "api" {
		t.Errorf("push facts %+v", pf)
	}
	// a second pass finds every head terminal: nothing enqueued, and the unchanged repository costs 304s
	n, before := len(r.enq.deliveries()), r.f.total()
	r.clk.Advance(SweepEvery)
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if len(r.enq.deliveries()) != n {
		t.Fatalf("second pass enqueued %d more", len(r.enq.deliveries())-n)
	}
	if after := r.f.total() - before; after > 3 { // hooks (403), repo (304), pulls (304)
		t.Errorf("idle pass made %d requests, want at most 3", after)
	}
}

func indexOf(e *recEnq, rev string) int {
	for i, d := range e.deliveries() {
		if d.Revision == rev {
			return i
		}
	}
	return -1
}

// TestRedeliverOnce proves acceptance line 2: with permission, each failed delivery of the last 72 h is
// redelivered once, a delivery that later succeeded or is older than 72 h is not, only the coordinator's
// hook is touched, no head scan runs, and a second pass redelivers none.
func TestRedeliverOnce(t *testing.T) {
	r := newRig(t, "acme/app")
	repo := r.f.repos["acme/app"]
	repo.Hooks = []fHook{
		{ID: 11, Active: true, Events: []string{"push", "pull_request"}, URL: "https://ci.example/webhook"},
		{ID: 12, Active: true, Events: []string{"push"}, URL: "https://other.example/hook"},
		{ID: 13, Active: false, Events: []string{"push"}, URL: "https://ci.example/old"},
	}
	repo.Deliveries[11] = []fDelivery{
		{ID: 901, GUID: "g1", Code: 500, AgeHrs: 2},   // failed: redeliver
		{ID: 902, GUID: "g2", Code: 200, AgeHrs: 3},   // fine
		{ID: 903, GUID: "g3", Code: 502, AgeHrs: 80},  // too old
		{ID: 904, GUID: "g4", Code: 500, AgeHrs: 10},  // failed, then
		{ID: 905, GUID: "g4", Code: 200, AgeHrs: 9},   // succeeded on a retry
		{ID: 906, GUID: "g5", Code: 0, AgeHrs: 1},     // never answered: failed
		{ID: 907, GUID: "g1", Code: 500, AgeHrs: 1.5}, // a failed retry of g1: g1 still once
	}
	repo.Deliveries[12] = []fDelivery{{ID: 951, GUID: "x1", Code: 500, AgeHrs: 1}}
	r.policy(map[string]string{KeyIngress: `"webhook"`, KeyHookURL: `"https://ci.example/"`})
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if strings.Join(repo.Redelivered, ",") != "906,907" { // the newest attempt of g5 and of g1, once each, oldest first
		t.Fatalf("redelivered %v, want [906 907]", repo.Redelivered)
	}
	if r.f.count(hasPath("/pulls")) != 0 || len(r.enq.deliveries()) != 0 {
		t.Error("a usable hook must not trigger the head scan")
	}
	r.clk.Advance(SweepEvery)
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if len(repo.Redelivered) != 2 {
		t.Fatalf("second pass redelivered more: %v", repo.Redelivered)
	}
	// without hook_url every active hook that covers push counts as the coordinator's
	r2 := newRig(t, "acme/app")
	r2.f.repos["acme/app"].Hooks = repo.Hooks
	r2.f.repos["acme/app"].Deliveries = map[int64][]fDelivery{12: repo.Deliveries[12]}
	r2.policy(map[string]string{KeyIngress: `"webhook"`})
	if err := r2.e.GitHub().Reconcile(context.Background(), r2.enq); err != nil {
		t.Fatal(err)
	}
	if got := r2.f.repos["acme/app"].Redelivered; len(got) != 1 || got[0] != "951" {
		t.Fatalf("no hook_url: redelivered %v, want 951", got)
	}
	// deliveries refused (404) fall back to the head scan
	r3 := newRig(t, "acme/app")
	r3.f.repos["acme/app"].Hooks = repo.Hooks
	r3.f.repos["acme/app"].DeliveryDeny = true
	r3.policy(map[string]string{KeyIngress: `"webhook"`})
	if err := r3.e.GitHub().Reconcile(context.Background(), r3.enq); err != nil {
		t.Fatal(err)
	}
	if len(r3.enq.deliveries()) != 1 {
		t.Fatalf("deliveries 404: %d enqueued, want the main head", len(r3.enq.deliveries()))
	}
}

// TestQuotaStops proves acceptance line 3: a quota error mid-pass stops the pass, records RetryAt, and the
// next tick before RetryAt makes no request; after RetryAt the sweep resumes. Without a stated time the
// backoff is 30 s x 2^n with the jitter centred, capped at 15 min.
func TestQuotaStops(t *testing.T) {
	r := newRig(t, "acme/app", "acme/lib")
	for _, name := range []string{"acme/app", "acme/lib"} {
		r.f.repos[name].Branches = map[string]string{"main": sha('a', len(name))}
	}
	r.f.quota = func(n int, req *http.Request) (int, int) {
		if n == 4 && strings.Contains(req.URL.Path, "/commits/") { // mid-pass, on a status read
			return 429, 120
		}
		return 0, 0
	}
	err := r.e.GitHub().Reconcile(context.Background(), r.enq)
	var q *QuotaStop
	if !errors.As(err, &q) {
		t.Fatalf("pass error = %v, want a QuotaStop", err)
	}
	want := r.clk.Now().Add(120 * time.Second)
	if d := q.RetryAt.Sub(want); d < -2*time.Second || d > 2*time.Second { // the funnel stamps Retry-After with the wall clock
		t.Fatalf("RetryAt = %v, want %v", q.RetryAt, want)
	}
	if r.f.count(func(q reqRec) bool { return strings.Contains(q.Path, "acme/lib") }) != 0 {
		t.Error("the pass went on to the second repository after a quota error")
	}
	before := r.f.total()
	r.clk.Advance(60 * time.Second)
	if err = r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil || r.f.total() != before {
		t.Fatalf("tick before RetryAt: err=%v requests=%d, want none", err, r.f.total()-before)
	}
	if err = r.e.Demand().Reconcile(context.Background(), r.enq); err != nil || r.f.total() != before {
		t.Fatalf("demand tick before RetryAt: err=%v requests=%d, want none (the quota stop is shared)", err, r.f.total()-before)
	}
	r.clk.Advance(61 * time.Second)
	r.f.quota = nil
	if err = r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if len(r.enq.deliveries()) != 2 {
		t.Fatalf("after RetryAt: %d enqueued, want both repositories' heads", len(r.enq.deliveries()))
	}
	// no stated time: 30 s, 60 s, 120 s ... capped at 15 min, jitter centred by the test
	e := NewEngine()
	e.Now, e.Jitter = r.clk.Now, func() float64 { return 0.5 }
	e.Use(newMemState())
	ctx := context.Background()
	for i, want := range []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 480 * time.Second, 900 * time.Second, 900 * time.Second} {
		if got := e.noteQuota(ctx, time.Time{}).Sub(r.clk.Now()); got != want {
			t.Errorf("backoff %d = %v, want %v", i, got, want)
		}
	}
	e.Jitter = func() float64 { return 0 }
	e.Use(newMemState())
	if got := e.noteQuota(ctx, time.Time{}).Sub(r.clk.Now()); got != 24*time.Second { // 30 s -20%
		t.Errorf("low jitter = %v, want 24s", got)
	}
}

// TestCallBound proves acceptance line 4: a repository with 150 open pull requests costs at most 200
// requests and scans 100 of them; a lower bound stops the pass with ErrCallBound and the next pass goes
// on from where the marks say.
func TestCallBound(t *testing.T) {
	r := newRig(t, "acme/big")
	repo := r.f.repos["acme/big"]
	for i := 1; i <= 150; i++ {
		repo.Pulls = append(repo.Pulls, fPull{Number: i, Head: fmt.Sprintf("%040x", 0xabc000+i), Assoc: "NONE"})
	}
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	prs := 0
	for i := range r.enq.deliveries() {
		if r.enq.facts(t, i).Kind == "pull_request" {
			prs++
		}
	}
	if prs != MaxPRs {
		t.Fatalf("scanned %d pull requests, want %d", prs, MaxPRs)
	}
	if n := r.f.total(); n > MaxPassCalls {
		t.Fatalf("pass made %d requests, bound is %d", n, MaxPassCalls)
	}
	// a low bound: the pass stops with ErrCallBound after exactly that many requests, then continues
	r2 := newRig(t, "acme/big")
	r2.f.repos["acme/big"].Pulls = repo.Pulls
	r2.e.PassCalls = 20
	err := r2.e.GitHub().Reconcile(context.Background(), r2.enq)
	if !errors.Is(err, ErrCallBound) || r2.f.total() != 20 {
		t.Fatalf("bounded pass: err=%v requests=%d, want ErrCallBound and 20", err, r2.f.total())
	}
	first := len(r2.enq.deliveries())
	r2.clk.Advance(SweepEvery)
	_ = r2.e.GitHub().Reconcile(context.Background(), r2.enq)
	if len(r2.enq.deliveries()) <= first {
		t.Fatalf("next pass made no progress: %d then %d", first, len(r2.enq.deliveries()))
	}
}

// TestPollHeadScan proves acceptance line 7: with no webhook configured, three pushes are enqueued by the
// head scan within one 5-minute pass and no hook endpoint is touched; an idle repository then costs two
// conditional requests; webhook mode with a reachable hook redelivers first and scans nothing.
func TestPollHeadScan(t *testing.T) {
	r := newRig(t, "acme/app")
	repo := r.f.repos["acme/app"]
	repo.Branches = map[string]string{"main": sha('a', 1), "release": sha('b', 2), "stable": sha('c', 3)}
	repo.Protected = []string{"release", "stable"}
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if len(r.enq.deliveries()) != 3 {
		t.Fatalf("enqueued %d heads, want 3", len(r.enq.deliveries()))
	}
	if r.f.count(hasPath("/hooks")) != 0 {
		t.Error("poll mode touched the hooks endpoints")
	}
	r.f.reset()
	r.clk.Advance(SweepEvery)
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if n := r.f.total(); n != 2 || r.f.nonCond304() != 0 {
		t.Errorf("idle poll pass: %d requests, %d not 304; want 2 and 0", n, r.f.nonCond304())
	}
	// a new push: the repository object changes, and only the new head is read and enqueued
	repo.Branches["stable"] = sha('d', 4)
	repo.Rev++
	r.clk.Advance(SweepEvery)
	if err := r.e.GitHub().Reconcile(context.Background(), r.enq); err != nil {
		t.Fatal(err)
	}
	if len(r.enq.deliveries()) != 4 || r.enq.deliveries()[3].Revision != sha('d', 4) {
		t.Fatalf("new push not enqueued: %d deliveries", len(r.enq.deliveries()))
	}
	// webhook mode, hook reachable: redelivery first, nothing scanned
	w := newRig(t, "acme/app")
	w.f.repos["acme/app"].Hooks = []fHook{{ID: 1, Active: true, Events: []string{"push"}, URL: "https://ci.example/webhook"}}
	w.f.repos["acme/app"].Deliveries[1] = []fDelivery{{ID: 5, GUID: "gg", Code: 500, AgeHrs: 1}}
	w.policy(map[string]string{KeyIngress: `"webhook"`})
	if err := w.e.GitHub().Reconcile(context.Background(), w.enq); err != nil {
		t.Fatal(err)
	}
	if got := w.f.repos["acme/app"].Redelivered; len(got) != 1 || w.f.count(hasPath("/pulls")) != 0 || len(w.enq.deliveries()) != 0 {
		t.Errorf("webhook mode: redelivered %v, pulls requests %d, enqueued %d", got, w.f.count(hasPath("/pulls")), len(w.enq.deliveries()))
	}
}
