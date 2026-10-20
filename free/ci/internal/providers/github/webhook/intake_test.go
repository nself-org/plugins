package webhook

// Purpose: end-to-end tests through the real NODE-10 intake (acceptance lines 1, 3 and 6): the source
// is registered under the runtime ingress adapter, requests go through the coordinator's HTTP handler
// over a real store, and the 403 / 200 accepted:false / 202 answers are the coordinator's own.
// Inputs:  the recorded deliveries, the fake GitHub API, a temporary store.
// Outputs: pass or fail.
// Constraints: the registries are process-wide, so the tests wire the registered Default source to
//          the fake API and restore it afterwards.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	_ "github.com/nself-org/plugins/free/ci/internal/providers/runtime" // registers the ingress adapter
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// wire points the registered Default source at rg's fake API, secrets and environment.
func wire(t *testing.T, rg *rig) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	was := Source{Now: Default.Now, Getenv: Default.Getenv, Secrets: Default.Secrets, Insecure: Default.Insecure, NewAPI: Default.NewAPI}
	Default.Now, Default.Getenv, Default.Secrets, Default.Insecure, Default.NewAPI = rg.src.Now, rg.src.Getenv, rg.src.Secrets, rg.src.Insecure, rg.src.NewAPI
	Default.pins = pinTable{}
	t.Cleanup(func() {
		Default.Now, Default.Getenv, Default.Secrets, Default.Insecure, Default.NewAPI = was.Now, was.Getenv, was.Secrets, was.Insecure, was.NewAPI
		Default.pins = pinTable{}
	})
}

type intakeRig struct {
	h  http.Handler
	st *store.Store
	t  *testing.T
}

func newIntake(t *testing.T, allowed ...string) *intakeRig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	c, err := coordinator.New(coordinator.Config{Store: st, AllowedRepos: allowed, PolicyDigest: "pd"})
	if err != nil {
		t.Fatal(err)
	}
	return &intakeRig{h: c.Handler(true), st: st, t: t}
}

// post sends a delivery; secret "" sends it unsigned.
func (i *intakeRig) post(event, delivery, secret string, body []byte) *httptest.ResponseRecorder {
	sig := ""
	if secret != "" {
		sig = sign(secret, body)
	}
	req := request(event, delivery, sig, body)
	w := httptest.NewRecorder()
	i.h.ServeHTTP(w, req)
	return w
}

func (i *intakeRig) queued() int {
	s, err := i.st.IntakeStats(context.Background())
	if err != nil {
		i.t.Fatal(err)
	}
	return s.Queued
}

func (i *intakeRig) facts(w *httptest.ResponseRecorder) model.TriggerFacts {
	i.t.Helper()
	var ans map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &ans); err != nil {
		i.t.Fatalf("answer %q: %v", w.Body, err)
	}
	row, err := i.st.IntakeGet(context.Background(), ans["intake"])
	if err != nil {
		i.t.Fatal(err)
	}
	var f model.TriggerFacts
	if err = json.Unmarshal(row.Facts, &f); err != nil {
		i.t.Fatal(err)
	}
	return f
}

type demandProbe struct {
	mu     sync.Mutex
	labels []string
}

func (d *demandProbe) Synthesize(_ context.Context, f model.TriggerFacts) (*model.Pipeline, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if f.RunnerLabels.Value != nil {
		d.labels = *f.RunnerLabels.Value
	}
	return &model.Pipeline{ID: "demand-1", Name: "demand", Kind: "runner_demand"}, nil
}

var (
	probe     = &demandProbe{}
	probeOnce sync.Once
)

// TestIntakeEndToEnd proves acceptance lines 1, 3 and 6 through NODE-10's intake: a fork PR signed
// with the repository's own secret is enqueued without an allowlist entry (the secret authorises its
// repository) and its stored facts reach a classifier through providers.RefetchFacts with the fork
// facts read from the API; a shared-secret delivery stays under --allowed-repos; a bad signature is
// 403 with nothing enqueued; a workflow_job queued reaches the demand handler.
func TestIntakeEndToEnd(t *testing.T) {
	rg := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	rg.env["SYNTH_REPO_SECRET"] = repoSecret
	rg.env[SharedSecretEnv] = sharedSecret
	wire(t, rg)
	probeOnce.Do(func() {
		if err := providers.RegisterDemandHandler("github", probe); err != nil {
			t.Fatal(err)
		}
	})
	in := newIntake(t, "o/shared")

	w := in.post("pull_request", "d-pr", repoSecret, fixture(t, "pr_fork_opened.json"))
	if w.Code != http.StatusAccepted {
		t.Fatalf("fork PR with its own secret = %d %s", w.Code, w.Body)
	}
	stored := in.facts(w)
	if stored.Auth != "per-repo" || stored.Repo != "o/r" || stored.Kind != "pull_request" {
		t.Fatalf("stored facts = %+v", stored)
	}
	// the classifier is the TRUST-13 hook: it sees what RefetchFacts returns
	var classified model.TriggerFacts
	classify := func(f model.TriggerFacts) { classified = f }
	fresh, err := providers.RefetchFacts(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	classify(fresh)
	if classified.Fork.Value == nil || !*classified.Fork.Value || classified.Fork.Source != "api" || classified.HeadRepo.Value == nil || *classified.HeadRepo.Value != "newbie/r" ||
		classified.ActorPermission.Value == nil || *classified.ActorPermission.Value != "read" || classified.FirstTime.Value == nil || !*classified.FirstTime.Value {
		t.Fatalf("the classifier must receive the fork PR facts read from the API: %+v", classified)
	}

	shared := replace(replace(fixture(t, "push.json"), `"full_name":"o/r"`, `"full_name":"o/shared"`), `"id":1001`, `"id":3003`)
	if w = in.post("push", "d-shared", sharedSecret, shared); w.Code != http.StatusAccepted || in.facts(w).Auth != "shared" {
		t.Fatalf("allowlisted shared-secret delivery = %d %s", w.Code, w.Body)
	}
	other := replace(replace(shared, `"full_name":"o/shared"`, `"full_name":"o/other"`), `"id":3003`, `"id":4004`)
	before := in.queued()
	if w = in.post("push", "d-other", sharedSecret, other); w.Code != http.StatusForbidden || in.queued() != before {
		t.Fatalf("shared-secret delivery outside --allowed-repos = %d, queued %d -> %d", w.Code, before, in.queued())
	}
	if w = in.post("pull_request", "d-bad", "synthetic-wrong-secret", fixture(t, "pr_fork_opened.json")); w.Code != http.StatusForbidden || in.queued() != before {
		t.Fatalf("bad signature = %d, queued %d -> %d", w.Code, before, in.queued())
	}
	if w = in.post("pull_request", "d-nosig", "", fixture(t, "pr_fork_opened.json")); w.Code != http.StatusForbidden || in.queued() != before {
		t.Fatalf("missing signature = %d", w.Code)
	}
	if w = in.post("pull_request", "d-closed", repoSecret, replace(fixture(t, "pr_fork_opened.json"), `"action":"opened"`, `"action":"closed"`)); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"accepted":false`) || in.queued() != before {
		t.Fatalf("closed PR = %d %s", w.Code, w.Body)
	}
	if w = in.post("workflow_job", "d-job", repoSecret, fixture(t, "workflow_job_queued.json")); w.Code != http.StatusAccepted || len(probe.labels) != 4 {
		t.Fatalf("workflow_job queued = %d %s labels %v", w.Code, w.Body, probe.labels)
	}
	if got := in.facts(w); got.Kind != "runner_demand" || got.Auth != "per-repo" {
		t.Fatalf("demand facts = %+v", got)
	}
}

// TestNoSelfDispatchLoop proves acceptance line 3 (review F3): a workflow_dispatch delivery of
// .github/workflows/nself-ci-dispatch.yml (the coordinator's own dispatch) and any other
// workflow_dispatch yield 200 accepted:false and no enqueue, after the signature is checked.
func TestNoSelfDispatchLoop(t *testing.T) {
	rg := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	rg.env["SYNTH_REPO_SECRET"] = repoSecret
	wire(t, rg)
	in := newIntake(t)
	own := fixture(t, "workflow_dispatch.json")
	foreign := bytes.Replace(own, []byte("nself-ci-dispatch.yml"), []byte("release.yml"), 1)
	for name, body := range map[string][]byte{"the coordinator's own dispatch": own, "any other workflow_dispatch": foreign} {
		w := in.post("workflow_dispatch", "d-loop-"+name[:3], repoSecret, body)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"accepted":false`) || in.queued() != 0 {
			t.Fatalf("%s = %d %s queued %d", name, w.Code, w.Body, in.queued())
		}
	}
	if rg.gh.calls() != 0 {
		t.Fatalf("a dropped event must not cost an API call, got %d", rg.gh.calls())
	}
	if w := in.post("workflow_dispatch", "d-loop-unsigned", "", own); w.Code != http.StatusForbidden {
		t.Fatalf("an unsigned workflow_dispatch must be 403 (nothing is learned before the signature), got %d", w.Code)
	}
}
