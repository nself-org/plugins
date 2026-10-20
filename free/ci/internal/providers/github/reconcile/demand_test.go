package reconcile

// Purpose: tests of poll-mode runner demand and the operator keys (P7-HOST-17 acceptance lines 5, 6, 8, 9
// and the F20 dedup line).
// Inputs:  the fixture forge of rig_test.go; a real store and coordinator for the dedup test.
// Outputs: pass or fail.
// Constraints: no live network; the clock is injected; every name is synthetic.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var nself = []string{"self-hosted", "nself"}

func tick(t *testing.T, r *rig) error {
	t.Helper()
	r.clk.Advance(DefaultPollInterval)
	return r.e.Demand().Reconcile(context.Background(), r.enq)
}

// TestPollDemand proves acceptance lines 5 and 8: a queued run whose job has [self-hosted, nself] yields
// exactly one handler call with facts sourced api and auth per-repo, the same job on the next tick none, a
// ubuntu-latest job none, and an idle tick one request; an in_progress run with one completed job and one
// queued [self-hosted, nself] job yields exactly one demand with RunEvent, HeadRepo and Fork read from the
// run object (and no Ref).
func TestPollDemand(t *testing.T) {
	r := newRig(t, "acme/app")
	repo := r.f.repos["acme/app"]
	if err := tick(t, r); err != nil || r.f.total() != 1 {
		t.Fatalf("idle first tick: err=%v requests=%d, want one", err, r.f.total())
	}
	r.f.reset()
	if err := tick(t, r); err != nil || r.f.total() != 1 || r.f.nonCond304() != 0 {
		t.Fatalf("idle tick: err=%v requests=%d (non-304 %d), want one conditional request", err, r.f.total(), r.f.nonCond304())
	}
	head := sha('1', 1)
	repo.Runs = []fRun{
		{ID: 5001, Status: "queued", Event: "push", Head: head, Jobs: []fJob{{ID: 7001, Status: "queued", Labels: nself}}},
		{ID: 5002, Status: "queued", Event: "push", Head: sha('2', 2), Jobs: []fJob{{ID: 7002, Status: "queued", Labels: []string{"ubuntu-latest"}}}},
	}
	if err := tick(t, r); err != nil {
		t.Fatal(err)
	}
	calls := handler.take()
	if len(calls) != 1 {
		t.Fatalf("%d handler calls, want 1 (the ubuntu-latest job asks for hosted capacity)", len(calls))
	}
	f := calls[0]
	if f.Kind != "runner_demand" || f.Source != "github" || f.Auth != "per-repo" || f.Repo != "acme/app" || f.Revision != head || f.Ref != "" {
		t.Errorf("facts %+v", f)
	}
	if f.RunnerLabels.Source != "api" || f.ProviderJobID.Source != "api" || *f.ProviderJobID.Value != "7001" || *f.ProviderRunID.Value != "5001" || f.RunEvent.Source != "api" {
		t.Errorf("fact provenance %+v", f)
	}
	ds := r.enq.deliveries()
	if len(ds) != 1 || ds[0].TriggerClass != "runner_demand" || ds[0].Synthetic == nil || ds[0].DeliveryID != "poll:job:7001" {
		t.Fatalf("deliveries %+v", ds)
	}
	if err := tick(t, r); err != nil || len(handler.take()) != 0 || len(r.enq.deliveries()) != 1 {
		t.Fatalf("same job on the next tick must yield none (err=%v)", err)
	}
	// the in_progress case: a job queued behind `needs:` inside a run that already started
	repo.Runs = []fRun{{ID: 5100, Status: "in_progress", Event: "pull_request", Head: sha('3', 3), HeadRepoID: 777, HeadRepoNam: "forker/app",
		Jobs: []fJob{{ID: 8001, Status: "completed", Labels: nself}, {ID: 8002, Status: "queued", Labels: nself}}}}
	for i := 0; i < inProgressEvery; i++ { // an idle repository reads the in_progress list on every 4th tick
		if err := tick(t, r); err != nil {
			t.Fatal(err)
		}
	}
	calls = handler.take()
	if len(calls) != 1 {
		t.Fatalf("in_progress run: %d handler calls, want 1", len(calls))
	}
	f = calls[0]
	if *f.ProviderJobID.Value != "8002" || f.RunEvent.Value == nil || *f.RunEvent.Value != "pull_request" || f.HeadRepo.Value == nil || *f.HeadRepo.Value != "forker/app" ||
		f.Fork.Value == nil || !*f.Fork.Value || f.HeadRepo.Source != "api" || f.Ref != "" || f.Actor.Value == nil || *f.Actor.Value != "octo" {
		t.Errorf("in_progress facts %+v", f)
	}
	// a handler that refuses is asked once, not every tick
	handler.mu.Lock()
	handler.refuse = true
	handler.mu.Unlock()
	repo.Runs = []fRun{{ID: 5200, Status: "queued", Event: "push", Head: sha('4', 4), Jobs: []fJob{{ID: 9001, Status: "queued", Labels: nself}}}}
	for i := 0; i < 3; i++ {
		if err := tick(t, r); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(handler.take()); n != 1 {
		t.Errorf("refusing handler was asked %d times, want 1", n)
	}
	// webhook mode: this Reconciler does nothing
	r.policy(map[string]string{KeyIngress: `"webhook"`})
	r.f.reset()
	repo.Runs = []fRun{{ID: 5300, Status: "queued", Event: "push", Head: sha('5', 5), Jobs: []fJob{{ID: 9101, Status: "queued", Labels: nself}}}}
	if err := tick(t, r); err != nil || r.f.total() != 0 {
		t.Errorf("webhook mode polled: err=%v requests=%d", err, r.f.total())
	}
}

// TestPollRateBudget proves acceptance line 9 (review F8): 10 repositories, poll mode, one simulated hour
// with no changes: every repeated GET is conditional and at most 300 are not 304; with the head scans
// deferred by the hourly ledger, demand polling never stops; a 429 with retry-after backs off and resumes
// after it.
func TestPollRateBudget(t *testing.T) {
	names := make([]string, 10)
	for i := range names {
		names[i] = "acme/r" + string(rune('a'+i))
	}
	r := newRig(t, names...)
	r.setTerminal()
	runHour := func(r *rig) {
		for s := 0; s < 3600; s += 30 {
			_ = r.e.Demand().Reconcile(context.Background(), r.enq)
			if s%300 == 0 {
				_ = r.e.GitHub().Reconcile(context.Background(), r.enq)
			}
			r.clk.Advance(30 * time.Second)
		}
	}
	runHour(r)
	if n := r.f.nonCond304(); n > 300 {
		t.Errorf("%d non-304 requests in the hour, want at most 300", n)
	}
	seen := map[string]bool{}
	r.f.mu.Lock()
	for _, q := range r.f.reqs {
		key := q.Method + " " + q.Path + "?" + q.Query
		if seen[key] && !q.Cond {
			t.Errorf("repeated unconditional request %s", key)
		}
		seen[key] = true
	}
	r.f.mu.Unlock()
	if len(r.enq.deliveries()) != 10 {
		t.Errorf("%d heads enqueued, want one per repository", len(r.enq.deliveries()))
	}

	// head scans deferred by the ledger (scans may use 60% of the allowance); demand goes on
	d := newRig(t, names...)
	d.e.HourlyBudget = 100
	runHour(d)
	lists := d.f.count(func(q reqRec) bool {
		return strings.HasSuffix(q.Path, "/actions/runs") && strings.Contains(q.Query, "status=queued")
	})
	if lists != 10*120 {
		t.Errorf("queued-run lists read %d times, want %d (one per repository per tick, never stopped)", lists, 10*120)
	}
	if got := len(d.enq.deliveries()); got == 0 || got >= 10*3 {
		t.Logf("deferred scans still enqueued %d heads", got)
	}
	if d.e.ledger.Scan > 60 {
		t.Errorf("scans spent %d of the hour's %d, the cap is 60%%", d.e.ledger.Scan, d.e.HourlyBudget)
	}

	// 429 with retry-after: back off, then resume
	b := newRig(t, "acme/app")
	b.f.quota = func(n int, req *http.Request) (int, int) {
		if n == 0 {
			return 429, 90
		}
		return 0, 0
	}
	err := b.e.Demand().Reconcile(context.Background(), b.enq)
	if err == nil || b.f.total() != 1 {
		t.Fatalf("429 tick: err=%v requests=%d", err, b.f.total())
	}
	b.clk.Advance(30 * time.Second)
	_ = b.e.Demand().Reconcile(context.Background(), b.enq)
	b.clk.Advance(30 * time.Second)
	_ = b.e.Demand().Reconcile(context.Background(), b.enq)
	if b.f.total() != 1 {
		t.Fatalf("polled during backoff: %d requests", b.f.total())
	}
	b.clk.Advance(31 * time.Second)
	if err = b.e.Demand().Reconcile(context.Background(), b.enq); err != nil || b.f.total() != 2 {
		t.Fatalf("after retry-after: err=%v requests=%d, want a request", err, b.f.total())
	}
}

// TestIngressKeys proves acceptance line 8's key rules: poll is the default, the keys are operator-only
// (a revision-scoped value is ignored with E671), poll_interval 5s is raised to 15s, bad values are E672,
// and the Reconciler cadence follows the setting.
func TestIngressKeys(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, spec := range Keys() {
		got, ok := policy.Default.Find(spec.Key)
		if !ok || !got.Spend || got.Kind != policy.TightenFixed {
			t.Errorf("%s: registered=%v operator-only=%v kind=%v", spec.Key, ok, got.Spend, got.Kind)
		}
	}
	def, err := SettingsFrom(&trust.Effective{})
	if err != nil || def.Ingress != IngressPoll || def.PollInterval != 30*time.Second {
		t.Fatalf("defaults %+v %v", def, err)
	}
	evil := map[string]string{KeyIngress: `"webhook"`, KeyPollInterval: `"1s"`, KeyHookURL: `"https://evil.example"`}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff := mergeScopes(t, scope(kind, evil))
		s, err := SettingsFrom(&eff)
		if err != nil || s != def {
			t.Fatalf("%s scope changed the settings: %+v %v", kind, s, err)
		}
		ignored := map[string]bool{}
		for _, ig := range eff.Ignored() {
			if !strings.HasPrefix(ig.Reason, "E671") {
				t.Errorf("%s: %s ignored with %q, want E671", kind, ig.Key, ig.Reason)
			}
			ignored[ig.Key] = true
		}
		for k := range evil {
			if !ignored[k] {
				t.Errorf("%s: %s missing from Effective.Ignored", kind, k)
			}
		}
	}
	eff := mergeScopes(t, scope(trust.SourceUser, map[string]string{KeyIngress: `"webhook"`, KeyPollInterval: `"5s"`, KeyHookURL: `"https://ci.example/hook/"`}))
	s, err := SettingsFrom(&eff)
	if err != nil || s.Ingress != IngressWebhook || s.PollInterval != 15*time.Second || s.HookURL != "https://ci.example/hook" || len(eff.Ignored()) != 0 {
		t.Fatalf("operator values: %+v %v ignored=%v", s, err, eff.Ignored())
	}
	for key, raw := range map[string]string{KeyIngress: `"push"`, KeyPollInterval: `"0s"`, KeyHookURL: `"http://ci.example"`} {
		if _, err := tryMerge(scope(trust.SourceUser, map[string]string{key: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s=%s: %v, want E672", key, raw, err)
		}
	}
	if _, err = SettingsFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never the defaults")
	}
	r := newRig(t, "acme/app")
	r.policy(map[string]string{KeyPollInterval: `"5s"`})
	if got := r.e.Demand().Every(); got != 15*time.Second {
		t.Errorf("Every() = %v, want 15s", got)
	}
	r.policy(nil)
	if got := r.e.Demand().Every(); got != 30*time.Second || r.e.GitHub().Every() != 5*time.Minute {
		t.Errorf("default cadence %v / %v", got, r.e.GitHub().Every())
	}
}

// TestWebhookAndPollDedup proves review F20: a webhook delivery and a head-scan enqueue of the same head,
// in either order, are one intake row (one pipeline).
func TestWebhookAndPollDedup(t *testing.T) {
	r := newRig(t, "acme/app", "acme/lib")
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	const secret = "synthetic-test-secret"
	c, err := coordinator.New(coordinator.Config{Store: st, Secret: secret, AllowedRepos: []string{"acme/app", "acme/lib"}, PolicyDigest: "pd"})
	if err != nil {
		t.Fatal(err)
	}
	push := func(repo, rev string) *httptest.ResponseRecorder {
		t.Setenv("NSELF_V15", "") // the v1.4 path verifies with the coordinator's own secret
		defer t.Setenv("NSELF_V15", "1")
		body, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": rev,
			"repository": map[string]string{"full_name": repo, "clone_url": "https://github.com/" + repo + ".git"}})
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "wh-"+repo)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
		w := httptest.NewRecorder()
		c.Handler(true).ServeHTTP(w, req)
		return w
	}
	appHead, libHead := r.f.repos["acme/app"].Branches["main"], r.f.repos["acme/lib"].Branches["main"]
	queued := func() int {
		s, err := st.IntakeStats(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return s.Queued
	}
	// the webhook arrives first, the sweep finds the same head
	if w := push("acme/app", appHead); w.Code != 202 {
		t.Fatalf("webhook = %d %s", w.Code, w.Body)
	}
	if err = r.e.GitHub().Reconcile(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := queued(); got != 2 { // app: webhook + sweep coalesced = 1; lib: sweep = 1
		t.Fatalf("queued rows = %d, want 2 (acme/app once, acme/lib once)", got)
	}
	// the sweep enqueues first, the webhook for the same head follows
	r.clk.Advance(SweepEvery)
	if w := push("acme/lib", libHead); w.Code != 202 || !strings.Contains(w.Body.String(), `"outcome":"coalesced"`) {
		t.Fatalf("late webhook = %d %s, want an accepted, coalesced answer", w.Code, w.Body)
	}
	if got := queued(); got != 2 {
		t.Fatalf("queued rows after the late webhook = %d, want 2", got)
	}
}
