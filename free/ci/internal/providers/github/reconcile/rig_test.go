package reconcile

// Purpose: the shared test rig of P7-HOST-17: a fixture GitHub forge over TLS (repositories, branches,
// pull requests, commit statuses, hooks and deliveries, workflow runs and jobs, ETags, injected quota
// errors), an in-memory State, a fake clock, a recording Enqueuer and a capturing DemandHandler.
// Inputs:  the tests.
// Outputs: rig values and request records the tests assert on.
// Constraints: no live network call (the forge binds loopback and the client trusts only its certificate);
//          every name, sha and token here is synthetic.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func sha(c byte, n int) string { return strings.Repeat(string(c), 39) + strconv.Itoa(n%10) }

// ---- the forge ----

type fPull struct {
	Number     int
	Head       string
	HeadRepoID int64 // 0: the base repository
	HeadName   string
	Assoc      string
}

type fHook struct {
	ID     int64
	Active bool
	Events []string
	URL    string
}

type fDelivery struct {
	ID     int64
	GUID   string
	Code   int
	AgeHrs float64
}

type fJob struct {
	ID     int64
	Status string
	Labels []string
}

type fRun struct {
	ID          int64
	Status      string
	Event       string
	Head        string
	HeadRepoID  int64
	HeadRepoNam string
	Jobs        []fJob
}

type fRepo struct {
	Name          string
	ID            int64
	Default       string
	Branches      map[string]string // branch -> sha
	Protected     []string
	Pulls         []fPull
	Status        map[string]string // sha -> nself-ci state
	Hooks         []fHook
	Deliveries    map[int64][]fDelivery
	Redelivered   []string
	RedeliverFail map[string]int // delivery id -> status the attempts endpoint answers with
	Runs          []fRun
	Rev           int
	HooksDenied   bool
	DeliveryDeny  bool
}

type reqRec struct {
	Method, Path, Query string
	Cond                bool
	Status              int
}

type forge struct {
	t     *testing.T
	srv   *httptest.Server
	mu    sync.Mutex
	repos map[string]*fRepo
	reqs  []reqRec
	// quota decides, per request, whether to answer with a quota error: status and Retry-After seconds.
	quota func(n int, r *http.Request) (status, retryAfter int)
	now   func() time.Time // the forge's clock, for delivery ages
}

func newForge(t *testing.T) *forge {
	f := &forge{t: t, repos: map[string]*fRepo{}, now: time.Now}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *forge) addRepo(name string, id int64) *fRepo {
	r := &fRepo{Name: name, ID: id, Default: "main", Branches: map[string]string{"main": sha('a', int(id))}, Status: map[string]string{}, Deliveries: map[int64][]fDelivery{}}
	f.mu.Lock()
	f.repos[strings.ToLower(name)] = r
	f.mu.Unlock()
	return r
}

func (f *forge) do(fn func()) { f.mu.Lock(); fn(); f.mu.Unlock() }

func (f *forge) count(match func(reqRec) bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, q := range f.reqs {
		if match(q) {
			n++
		}
	}
	return n
}

func (f *forge) total() int { return f.count(func(reqRec) bool { return true }) }

func (f *forge) nonCond304() int {
	return f.count(func(q reqRec) bool { return q.Status != 304 })
}

func (f *forge) reset() { f.do(func() { f.reqs = nil }) }

func (f *forge) client(context.Context) (*rest.Client, error) {
	return &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, Provider: "github", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig}, nil
}

func jsonOut(w http.ResponseWriter, r *http.Request, code int, v any) int {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	etag := `W/"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag && code == 200 {
		w.WriteHeader(http.StatusNotModified)
		return http.StatusNotModified
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
	return code
}

func (f *forge) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := reqRec{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Cond: r.Header.Get("If-None-Match") != ""}
	rec.Status = f.route(w, r)
	f.reqs = append(f.reqs, rec)
}

func (f *forge) route(w http.ResponseWriter, r *http.Request) int {
	if f.quota != nil {
		if st, ra := f.quota(len(f.reqs), r); st != 0 {
			w.Header().Set("Retry-After", strconv.Itoa(ra))
			return jsonOut(w, r, st, map[string]string{"message": "rate limited"})
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "repos" {
		return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
	}
	repo, ok := f.repos[strings.ToLower(parts[1]+"/"+parts[2])]
	if !ok {
		return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
	}
	rest := parts[3:]
	q := r.URL.Query()
	switch {
	case len(rest) == 0:
		return jsonOut(w, r, 200, map[string]any{"id": repo.ID, "full_name": repo.Name, "default_branch": repo.Default, "private": false, "visibility": "public", "rev": repo.Rev})
	case rest[0] == "branches" && len(rest) == 1:
		var out []map[string]any
		for _, b := range repo.Protected {
			out = append(out, map[string]any{"name": b, "protected": true, "commit": map[string]string{"sha": repo.Branches[b]}})
		}
		return jsonOut(w, r, 200, out)
	case rest[0] == "branches":
		name := strings.Join(rest[1:], "/")
		return jsonOut(w, r, 200, map[string]any{"name": name, "protected": false, "commit": map[string]string{"sha": repo.Branches[name]}})
	case rest[0] == "pulls":
		out := []map[string]any{}
		for _, p := range repo.Pulls {
			var hr any = map[string]any{"id": repo.ID, "full_name": repo.Name}
			if p.HeadRepoID > 0 {
				hr = map[string]any{"id": p.HeadRepoID, "full_name": p.HeadName}
			} else if p.HeadRepoID < 0 {
				hr = nil // the fork was deleted
			}
			out = append(out, map[string]any{"number": p.Number, "author_association": p.Assoc,
				"head": map[string]any{"sha": p.Head, "repo": hr}, "base": map[string]any{"ref": "main", "sha": repo.Branches["main"]}})
		}
		return jsonOut(w, r, 200, out)
	case rest[0] == "commits" && len(rest) == 3 && rest[2] == "status":
		st := repo.Status[rest[1]]
		out := map[string]any{"statuses": []any{}}
		if st != "" {
			out["statuses"] = []map[string]string{{"context": "nself-ci", "state": st}}
		}
		return jsonOut(w, r, 200, out)
	case rest[0] == "hooks":
		return f.hooks(w, r, repo, rest[1:])
	case rest[0] == "actions" && len(rest) >= 2 && rest[1] == "runs":
		return f.runs(w, r, repo, rest[2:], q.Get("status"))
	}
	return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
}

func (f *forge) hooks(w http.ResponseWriter, r *http.Request, repo *fRepo, rest []string) int {
	if repo.HooksDenied {
		return jsonOut(w, r, 403, map[string]string{"message": "Must have admin rights to Repository."})
	}
	if len(rest) == 0 {
		out := []map[string]any{}
		for _, h := range repo.Hooks {
			out = append(out, map[string]any{"id": h.ID, "active": h.Active, "events": h.Events, "config": map[string]string{"url": h.URL}})
		}
		return jsonOut(w, r, 200, out)
	}
	id, _ := strconv.ParseInt(rest[0], 10, 64)
	if repo.DeliveryDeny {
		return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
	}
	if len(rest) == 2 && rest[1] == "deliveries" {
		out := []map[string]any{}
		for _, d := range repo.Deliveries[id] {
			at := f.now().Add(-time.Duration(d.AgeHrs * float64(time.Hour)))
			out = append(out, map[string]any{"id": d.ID, "guid": d.GUID, "status_code": d.Code, "delivered_at": at.UTC().Format(time.RFC3339)})
		}
		return jsonOut(w, r, 200, out)
	}
	if len(rest) == 4 && rest[3] == "attempts" && r.Method == http.MethodPost {
		if code := repo.RedeliverFail[rest[2]]; code != 0 {
			return jsonOut(w, r, code, map[string]string{"message": "refused"})
		}
		repo.Redelivered = append(repo.Redelivered, rest[2])
		return jsonOut(w, r, 202, map[string]string{})
	}
	return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
}

func (f *forge) runs(w http.ResponseWriter, r *http.Request, repo *fRepo, rest []string, status string) int {
	if len(rest) == 0 {
		out := []map[string]any{}
		for _, run := range repo.Runs {
			if run.Status != status {
				continue
			}
			o := map[string]any{"id": run.ID, "status": run.Status, "event": run.Event, "head_sha": run.Head, "updated_at": "2026-10-19T12:00:00Z",
				"actor": map[string]string{"login": "octo"}, "repository": map[string]any{"id": repo.ID, "full_name": repo.Name, "private": false}}
			if run.HeadRepoNam != "" {
				o["head_repository"] = map[string]any{"id": run.HeadRepoID, "full_name": run.HeadRepoNam}
			}
			out = append(out, o)
		}
		return jsonOut(w, r, 200, map[string]any{"total_count": len(out), "workflow_runs": out})
	}
	id, _ := strconv.ParseInt(rest[0], 10, 64)
	for _, run := range repo.Runs {
		if run.ID == id && len(rest) == 2 && rest[1] == "jobs" {
			jobs := []map[string]any{}
			for _, j := range run.Jobs {
				jobs = append(jobs, map[string]any{"id": j.ID, "status": j.Status, "labels": j.Labels})
			}
			return jsonOut(w, r, 200, map[string]any{"total_count": len(jobs), "jobs": jobs})
		}
	}
	return jsonOut(w, r, 404, map[string]string{"message": "Not Found"})
}

// ---- state, clock, enqueuer ----

type memState struct {
	mu    sync.Mutex
	marks map[string]time.Time
	facts map[string]StoredFact
}

func newMemState() *memState {
	return &memState{marks: map[string]time.Time{}, facts: map[string]StoredFact{}}
}

func (m *memState) ProviderMarkGet(_ context.Context, name, key string) (time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.marks[name+"|"+key]
	return at, ok, nil
}

func (m *memState) ProviderMarkPut(_ context.Context, name, key string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marks[name+"|"+key] = at
	return nil
}

func (m *memState) ProviderFactGet(_ context.Context, p, s, k string) (StoredFact, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.facts[p+"|"+s+"|"+k]
	return f, ok, nil
}

func (m *memState) ProviderFactPut(_ context.Context, f StoredFact) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.facts[f.Provider+"|"+f.Scope+"|"+f.Key] = f
	return true, nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type recEnq struct {
	mu   sync.Mutex
	got  []coordinator.Delivery
	hook func(coordinator.Delivery)
}

func (r *recEnq) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	r.mu.Lock()
	r.got = append(r.got, d)
	hook := r.hook
	r.mu.Unlock()
	if hook != nil {
		hook(d)
	}
	return coordinator.Outcome{Kind: "enqueued", ID: fmt.Sprintf("in-%d", len(r.got))}, nil
}

func (r *recEnq) deliveries() []coordinator.Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]coordinator.Delivery(nil), r.got...)
}

func (r *recEnq) facts(t *testing.T, i int) model.TriggerFacts {
	t.Helper()
	var f model.TriggerFacts
	if err := json.Unmarshal(r.deliveries()[i].Facts, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// ---- the demand handler ----

var handler = &captureHandler{}

func init() {
	if err := providers.RegisterDemandHandler(Source, handler); err != nil {
		panic(err)
	}
}

type captureHandler struct {
	mu     sync.Mutex
	calls  []model.TriggerFacts
	refuse bool // answer with a providers.Error that has a code: a refusal
	fail   bool // answer with a plain error: a transient failure
}

func (h *captureHandler) Synthesize(_ context.Context, f model.TriggerFacts) (*model.Pipeline, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, f)
	if h.refuse {
		return nil, providers.NewError(providers.Config, "E708", "label.not_allowed", 0, time.Time{})
	}
	if h.fail {
		return nil, fmt.Errorf("capacity store unavailable")
	}
	return &model.Pipeline{ID: "demand-" + f.DeliveryID, Name: "demand"}, nil
}

func (h *captureHandler) take() []model.TriggerFacts {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.calls
	h.calls = nil
	return out
}

// ---- the rig ----

type rig struct {
	t   *testing.T
	f   *forge
	e   *Engine
	st  *memState
	clk *clock
	enq *recEnq
}

// newRig builds an Engine on a fresh forge. V15 is on and the demand handler starts clean.
func newRig(t *testing.T, repos ...string) *rig {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	handler.take()
	handler.mu.Lock()
	handler.refuse, handler.fail = false, false
	handler.mu.Unlock()
	f := newForge(t)
	for i, name := range repos {
		f.addRepo(name, int64(100+i))
	}
	clk := &clock{t: time.Now().UTC().Truncate(time.Second)} // the funnel stamps Retry-After with the wall clock
	st := newMemState()
	e := NewEngine()
	e.Now, e.NewAPI, e.Jitter = clk.Now, f.client, func() float64 { return 0.5 }
	e.Repos = func(context.Context) ([]string, error) { return repos, nil }
	e.Use(st)
	f.now = clk.Now
	rg := &rig{t: t, f: f, e: e, st: st, clk: clk, enq: &recEnq{}}
	rg.policy(nil)
	t.Cleanup(func() { UsePolicy(nil) })
	return rg
}

// policy installs an operator-scope policy with the given key values (JSON texts); nil means defaults.
func (r *rig) policy(kv map[string]string) {
	r.t.Helper()
	if kv == nil {
		UsePolicy(nil)
		return
	}
	eff := mergeScopes(r.t, scope(trust.SourceUser, kv))
	UsePolicy(func(context.Context) (*policy.Effective, error) { return &eff, nil })
}

func scope(kind trust.Source, kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: kind, Values: vals}
}

func mergeScopes(t *testing.T, scopes ...trust.Scope) trust.Effective {
	t.Helper()
	eff, err := tryMerge(scopes...)
	if err != nil {
		t.Fatal(err)
	}
	return eff
}

func tryMerge(scopes ...trust.Scope) (trust.Effective, error) {
	reg, err := trust.NewRegistry(append(Keys(), jitKeys()...)...)
	if err != nil {
		return trust.Effective{}, err
	}
	eff, _, err := reg.Merge("", scopes...)
	return eff, err
}

func jitKeys() []policy.KeySpec { return jit.Keys() }
