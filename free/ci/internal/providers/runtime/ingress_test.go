package runtime

// Purpose: tests of the ingress adapter against the real coordinator intake (P7-HOST-16, acceptance
// lines 4 and 7, review F4).
// Inputs:  the fake trigger source and a real coordinator over a real store, driven with httptest.
// Outputs: pass or fail.
// Constraints: the adapter registers itself with the coordinator at init, so these tests use that
//          registration; sources register once per process (the registry has no unregister).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// demandSrc is a second source, "fake-demand": it has no demand handler registered.
type demandSrc struct{ *hostedProv }

func (demandSrc) Name() string               { return "fake-demand" }
func (demandSrc) Match(r *http.Request) bool { return r.Header.Get("X-Test-Source") == "fake-demand" }
func (d demandSrc) Verify(ctx context.Context, r *http.Request, b []byte) (model.TriggerFacts, error) {
	f, err := d.hostedProv.Verify(ctx, r, b)
	f.Source = "fake-demand"
	return f, err
}

var demandOnce sync.Once

// pipelineMaker is the fake provider's demand handler.
type pipelineMaker struct {
	mu    sync.Mutex
	calls int
	out   *model.Pipeline
	err   error
}

func (p *pipelineMaker) Synthesize(context.Context, model.TriggerFacts) (*model.Pipeline, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.out, p.err
}

var maker = &pipelineMaker{}

func intakeRig(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st := openStore(t, nil)
	c, err := coordinator.New(coordinator.Config{Store: st, Secret: "synthetic-shared-secret", AllowedRepos: []string{"o/r"}, PolicyDigest: "pd"})
	if err != nil {
		t.Fatal(err)
	}
	return c.Handler(true), st
}

func post(h http.Handler, source, kind, delivery string, signed bool, extra ...string) *httptest.ResponseRecorder {
	req, body := prov.SignedTriggerFixture()
	if !signed {
		req.Header.Del("X-Hub-Signature-256")
	}
	req2 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req2.Header = req.Header.Clone()
	req2.Header.Set("X-Test-Source", source)
	req2.Header.Set("X-Test-Kind", kind)
	req2.Header.Set("X-Test-Delivery", delivery)
	for i := 0; i+1 < len(extra); i += 2 {
		req2.Header.Set(extra[i], extra[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req2)
	return w
}

func queuedCount(t *testing.T, st *store.Store) int {
	t.Helper()
	s, err := st.IntakeStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s.Queued
}

// TestIngressAdapter proves acceptance lines 4 and 7. A registered fake TriggerSource receives a POST
// through the coordinator's intake and the delivery row carries the TriggerFacts JSON; a Verify
// error is 403 with nothing enqueued; a runner_demand reaches the registered DemandHandler exactly
// once and is enqueued with a synthetic pipeline; with no handler, or a handler that refuses, the
// answer is 200 accepted:false with the reason and nothing is enqueued.
func TestIngressAdapter(t *testing.T) {
	useProvider(t)
	demandOnce.Do(func() {
		if err := providers.RegisterSource(demandSrc{prov}); err != nil {
			t.Fatal(err)
		}
		if err := providers.RegisterDemandHandler("fake", maker); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("verified delivery is enqueued with its facts", func(t *testing.T) {
		h, st := intakeRig(t)
		w := post(h, "fake", "", "d-1", true)
		if w.Code != 202 {
			t.Fatalf("verified delivery = %d %s", w.Code, w.Body)
		}
		var ans map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &ans); err != nil {
			t.Fatal(err)
		}
		row, err := st.IntakeGet(context.Background(), ans["intake"])
		if err != nil {
			t.Fatal(err)
		}
		var facts model.TriggerFacts
		if err = json.Unmarshal(row.Facts, &facts); err != nil || facts.Schema != "ci.trigger-facts/v1" || facts.Kind != "push" || facts.Repo != "o/r" || row.Source != "fake" || row.DeliveryID != "d-1" || row.TriggerClass != "change" {
			t.Fatalf("the delivery row must carry the TriggerFacts JSON: row=%+v facts=%+v err=%v", row, facts, err)
		}
	})
	t.Run("a Verify error is 403 and enqueues nothing", func(t *testing.T) {
		h, st := intakeRig(t)
		if w := post(h, "fake", "", "d-2", false); w.Code != 403 || queuedCount(t, st) != 0 {
			t.Fatalf("unsigned delivery = %d, queued=%d", w.Code, queuedCount(t, st))
		}
	})
	t.Run("facts naming another source are refused", func(t *testing.T) {
		h, st := intakeRig(t)
		if w := post(h, "fake", "", "d-forge", true, "X-Test-Facts-Source", "github"); w.Code != 403 || queuedCount(t, st) != 0 {
			t.Fatalf("forged source = %d, queued=%d", w.Code, queuedCount(t, st))
		}
	})
	t.Run("a request no source claims is not ours", func(t *testing.T) {
		if IngressAdapter.Match(IngressAdapter{}, httptest.NewRequest(http.MethodPost, "/webhook", nil)) {
			t.Fatal("the adapter matched a request with no source header")
		}
	})
	t.Run("runner_demand reaches the handler exactly once and is synthetic", func(t *testing.T) {
		h, st := intakeRig(t)
		maker.mu.Lock()
		maker.calls, maker.err = 0, nil
		maker.out = &model.Pipeline{ID: "demand-1", Name: "demand", Kind: "runner_demand"}
		maker.mu.Unlock()
		w := post(h, "fake", "runner_demand", "d-3", true)
		if w.Code != 202 || maker.calls != 1 || queuedCount(t, st) != 1 {
			t.Fatalf("runner_demand = %d %s calls=%d queued=%d", w.Code, w.Body, maker.calls, queuedCount(t, st))
		}
		var ans map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &ans)
		row, _ := st.IntakeGet(context.Background(), ans["intake"])
		if !strings.Contains(string(row.Facts), `"synthetic"`) || row.TriggerClass != "runner_demand" {
			t.Fatalf("the synthetic pipeline must travel with the delivery: class=%s facts=%s", row.TriggerClass, row.Facts)
		}
	})
	t.Run("runner_demand with no handler is 200 accepted:false", func(t *testing.T) {
		h, st := intakeRig(t)
		w := post(h, "fake-demand", "runner_demand", "d-4", true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":false`) || !strings.Contains(w.Body.String(), "no demand handler") || queuedCount(t, st) != 0 {
			t.Fatalf("no handler = %d %s queued=%d", w.Code, w.Body, queuedCount(t, st))
		}
	})
	t.Run("a refusing handler is 200 accepted:false without its error text", func(t *testing.T) {
		h, st := intakeRig(t)
		maker.mu.Lock()
		maker.err = providers.NewError(providers.Config, "E708", "raw detail synthetic-secret-123", 0, time.Time{})
		maker.mu.Unlock()
		w := post(h, "fake", "runner_demand", "d-5", true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":false`) || !strings.Contains(w.Body.String(), "E708") || strings.Contains(w.Body.String(), "synthetic-secret-123") || queuedCount(t, st) != 0 {
			t.Fatalf("refusing handler = %d %s queued=%d", w.Code, w.Body, queuedCount(t, st))
		}
	})
	t.Run("a handler that returns no pipeline is declined", func(t *testing.T) {
		h, st := intakeRig(t)
		maker.mu.Lock()
		maker.err, maker.out = nil, nil
		maker.mu.Unlock()
		if w := post(h, "fake", "runner_demand", "d-6", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":false`) || queuedCount(t, st) != 0 {
			t.Fatalf("nil pipeline = %d %s queued=%d", w.Code, w.Body, queuedCount(t, st))
		}
	})
	t.Run("allowlist precedes demand synthesis and decline", func(t *testing.T) {
		for _, source := range []string{"fake", "fake-demand"} {
			h, st := intakeRig(t)
			maker.mu.Lock()
			maker.calls = 0
			maker.mu.Unlock()
			w := post(h, source, "runner_demand", "outside", true, "X-Test-Repo", "outside/repo")
			if w.Code != 403 || maker.calls != 0 || queuedCount(t, st) != 0 {
				t.Fatalf("source=%s status=%d calls=%d", source, w.Code, maker.calls)
			}
		}
	})
	t.Run("dormant without V15", func(t *testing.T) {
		t.Setenv("NSELF_V15", "")
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("X-Test-Source", "fake")
		if (IngressAdapter{}).Match(req) || (IngressAdapter{}).HasSecrets() {
			t.Fatal("v1.4 mode must not route requests to provider sources")
		}
	})
}
