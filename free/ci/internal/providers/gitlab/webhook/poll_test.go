package webhook

// Purpose: prove the pending-job poll (D-1389) over a fake GitLab jobs API that honours If-None-Match: a
// capped demand is offered again and runs once a slot frees, a job is handed over at most once per status,
// nothing is read without a configured project, an unchanged list costs one conditional request and
// nothing more, and a rate-limited list stops the tick.
// Inputs:  scripted job lists per tick, a capped fake DemandHandler, a recording Enqueuer.
// Outputs: test assertions on enqueued job ids, handler calls and requests seen.
// Constraints: no live GitLab; NSELF_V15 is set for the poll to run.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const pollSHA = "0123456789abcdef0123456789abcdef01234567"

// jobsAPI serves /projects/{id}/jobs per project with an ETag of its body; 304 on a matching If-None-Match.
type jobsAPI struct {
	mu     sync.Mutex
	bodies map[string]string // escaped project ref -> body ("" = 404)
	status map[string]int    // forced status (429)
	reqs   []string          // "<ref> <200|304|...>"
	srv    *httptest.Server
}

func newJobsAPI(t *testing.T) *jobsAPI {
	a := &jobsAPI{bodies: map[string]string{}, status: map[string]int{}}
	a.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.EscapedPath(), "/projects/"), "/jobs")
		if r.URL.Query().Get("scope[]") != "pending" {
			t.Errorf("query = %s, want scope[]=pending", r.URL.RawQuery)
		}
		code, body := a.status[ref], a.bodies[ref]
		sum := sha256.Sum256([]byte(body))
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		switch {
		case code != 0:
		case body == "":
			code = 404
		case r.Header.Get("If-None-Match") == etag:
			code = 304
		default:
			code = 200
		}
		a.reqs = append(a.reqs, fmt.Sprintf("%s %d", ref, code))
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code == 200 {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func (a *jobsAPI) client(context.Context) (*rest.Client, error) {
	return &rest.Client{BaseURL: a.srv.URL, AllowPrivateBase: true, Provider: Name,
		TLSConfig: a.srv.Client().Transport.(*http.Transport).TLSClientConfig}, nil
}

func (a *jobsAPI) take() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.reqs
	a.reqs = nil
	return out
}

// pending renders a job list: "101" is a pending nself job, "101:running" another status, "101:linux" a job without our tag.
func pending(jobs ...string) string {
	var out []map[string]any
	for _, j := range jobs {
		id, extra, _ := strings.Cut(j, ":")
		status, tags := "pending", []string{"nself"}
		switch extra {
		case "linux":
			tags = []string{"linux"}
		case "":
		default:
			status = extra
		}
		var n int64
		_, _ = fmt.Sscan(id, &n)
		out = append(out, map[string]any{"id": n, "status": status, "tag_list": tags, "user": map[string]string{"username": "dev"},
			"pipeline": map[string]any{"id": 9, "project_id": 77, "sha": pollSHA}})
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

// capHandler admits at most max live demands and refuses the ids in refuse for good.
type capHandler struct {
	max    int
	live   map[string]bool
	refuse map[string]bool
	calls  map[string]int
}

func (h *capHandler) Synthesize(_ context.Context, f model.TriggerFacts) (*model.Pipeline, error) {
	id := *f.ProviderJobID.Value
	h.calls[id]++
	if f.Source != Name || f.Kind != "runner_demand" || f.Auth != "per-repo" || f.RunnerLabels.Source != "api" || f.Revision != pollSHA {
		return nil, fmt.Errorf("unexpected facts %+v", f)
	}
	if h.refuse[id] {
		return nil, providers.NewError(providers.Config, "E708", "label.not_allowed", 0, time.Time{})
	}
	if !h.live[id] && len(h.live) >= h.max {
		return nil, providers.NewError(providers.Quota, "E703", "runner.demand_capped", 0, time.Time{})
	}
	h.live[id] = true
	return &model.Pipeline{}, nil
}

type recorder struct{ ids []string }

func (r *recorder) Enqueue(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
	if d.Source != Name || d.TriggerClass != "runner_demand" || d.Synthetic == nil || !strings.HasPrefix(d.DeliveryID, "poll:job:") {
		return coordinator.Outcome{}, fmt.Errorf("unexpected delivery %+v", d)
	}
	r.ids = append(r.ids, strings.TrimPrefix(d.DeliveryID, "poll:job:"))
	return coordinator.Outcome{}, nil
}

// tick is one poll pass: set the lists (nil keeps them), free slots, then expect enqueued ids and requests.
type tick struct {
	lists    map[string]string
	status   map[string]int
	free     []string
	wantEnq  []string
	wantReqs []string
}

func TestPollTicks(t *testing.T) {
	signed := Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv}}}
	for _, tc := range []struct {
		name   string
		pol    Secrets
		max    int
		refuse []string
		ticks  []tick
		calls  map[string]int // handler calls per job at the end (nil: not checked)
	}{
		{name: "capped demand is offered again and runs once a slot frees", pol: signed, max: 1, ticks: []tick{
			{lists: map[string]string{"acme%2Fapp": pending("101", "102")}, wantEnq: []string{"101"}, wantReqs: []string{"acme%2Fapp 200"}},
			{wantReqs: []string{"acme%2Fapp 304"}}, // 102 still capped: offered, not marked
			{free: []string{"101"}, wantEnq: []string{"102"}, wantReqs: []string{"acme%2Fapp 304"}},
			{wantReqs: []string{"acme%2Fapp 304"}},
		}, calls: map[string]int{"101": 1, "102": 3}},
		{name: "a job is handed over at most once per status", pol: signed, max: 5, refuse: []string{"202"}, ticks: []tick{
			{lists: map[string]string{"acme%2Fapp": pending("201", "202", "203:linux")}, wantEnq: []string{"201"}, wantReqs: []string{"acme%2Fapp 200"}},
			{lists: map[string]string{"acme%2Fapp": pending("201", "202", "204:running")}, wantReqs: []string{"acme%2Fapp 200"}},
			{wantReqs: []string{"acme%2Fapp 304"}},
		}, calls: map[string]int{"201": 1, "202": 1}},
		{name: "no configured project reads nothing", pol: Secrets{}, max: 1, ticks: []tick{
			{lists: map[string]string{"acme%2Fapp": pending("301")}},
			{},
		}, calls: map[string]int{}},
		{name: "an unchanged list costs one conditional request", pol: signed, max: 5, ticks: []tick{
			{lists: map[string]string{"acme%2Fapp": pending("401")}, wantEnq: []string{"401"}, wantReqs: []string{"acme%2Fapp 200"}},
			{wantReqs: []string{"acme%2Fapp 304"}},
			{wantReqs: []string{"acme%2Fapp 304"}},
		}, calls: map[string]int{"401": 1}},
		{name: "a rate-limited list stops the tick", max: 5,
			pol: Secrets{Tokens: map[string]RepoSecret{"acme/a": {Env: tokenEnv}, "acme/b": {Env: tokenEnv}}}, ticks: []tick{
				{lists: map[string]string{"acme%2Fa": pending("501"), "acme%2Fb": pending("502")}, status: map[string]int{"acme%2Fa": 429},
					wantReqs: []string{"acme%2Fa 429"}},
				{status: map[string]int{}, wantEnq: []string{"501", "502"}, wantReqs: []string{"acme%2Fa 200", "acme%2Fb 200"}},
			}},
		{name: "a configured id addresses the project; another project id offers nothing", max: 5,
			pol: Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv, ID: 78}}}, ticks: []tick{
				{lists: map[string]string{"78": pending("601")}, wantReqs: []string{"78 200"}},
			}, calls: map[string]int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			api := newJobsAPI(t)
			h := &capHandler{max: tc.max, live: map[string]bool{}, refuse: map[string]bool{}, calls: map[string]int{}}
			for _, id := range tc.refuse {
				h.refuse[id] = true
			}
			apiBuilt := 0
			src := &Source{Now: func() time.Time { return fixedNow }, Policy: func(context.Context) (Secrets, error) { return tc.pol, nil },
				NewAPI: func(ctx context.Context) (*rest.Client, error) { apiBuilt++; return api.client(ctx) }}
			p := NewPoll(src)
			p.Handler = func(string) providers.DemandHandler { return h }
			for i, tk := range tc.ticks {
				api.mu.Lock()
				for k, v := range tk.lists {
					api.bodies[k] = v
				}
				if tk.status != nil {
					api.status = tk.status
				}
				api.mu.Unlock()
				for _, id := range tk.free {
					delete(h.live, id)
				}
				rec := &recorder{}
				_ = p.Reconcile(context.Background(), rec)
				if !slices.Equal(rec.ids, tk.wantEnq) {
					t.Fatalf("tick %d: enqueued %v, want %v", i, rec.ids, tk.wantEnq)
				}
				if got := api.take(); !slices.Equal(got, tk.wantReqs) {
					t.Fatalf("tick %d: requests %v, want %v", i, got, tk.wantReqs)
				}
			}
			if tc.calls != nil && !mapsEqual(h.calls, tc.calls) {
				t.Fatalf("handler calls %v, want %v", h.calls, tc.calls)
			}
			if len(tc.pol.Signing)+len(tc.pol.Tokens) == 0 && apiBuilt != 0 {
				t.Fatalf("API client built %d times with no project configured", apiBuilt)
			}
		})
	}
}

func TestPollIdentityMismatchIsAnError(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	api := newJobsAPI(t)
	api.bodies["78"] = pending("601") // pipeline.project_id is 77
	src := &Source{Now: func() time.Time { return fixedNow }, NewAPI: api.client,
		Policy: func(context.Context) (Secrets, error) {
			return Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv, ID: 78}}}, nil
		}}
	p := NewPoll(src)
	h := &capHandler{max: 5, live: map[string]bool{}, refuse: map[string]bool{}, calls: map[string]int{}}
	p.Handler = func(string) providers.DemandHandler { return h }
	if err := p.Reconcile(context.Background(), &recorder{}); err == nil || !strings.Contains(err.Error(), "bound to 78") {
		t.Fatalf("Reconcile = %v, want an identity error", err)
	}
}

func TestPollWithoutHandlerReadsNothing(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	api := newJobsAPI(t)
	src := &Source{Now: func() time.Time { return fixedNow }, NewAPI: api.client,
		Policy: func(context.Context) (Secrets, error) { return both(), nil }}
	p := NewPoll(src)
	p.Handler = func(string) providers.DemandHandler { return nil }
	if err := p.Reconcile(context.Background(), &recorder{}); err != nil {
		t.Fatal(err)
	}
	if got := api.take(); len(got) != 0 {
		t.Fatalf("requests %v, want none", got)
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
