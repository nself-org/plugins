package webhook

// Purpose: end-to-end tests through the real NODE-10 intake (acceptance lines 1 and 3): the source is
// registered under the runtime ingress adapter, requests go through the coordinator's HTTP handler over
// a real store, and the 403 / 200 accepted:false / 202 answers are the coordinator's own.
// Inputs:  the recorded deliveries, the fake GitLab API, a temporary store.
// Outputs: pass or fail.
// Constraints: the registries are process-wide, so the tests wire the registered Default source to the
//          fake API and restore it afterwards.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	_ "github.com/nself-org/plugins/free/ci/internal/providers/runtime" // registers the ingress adapter
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// wire points the registered Default source at rg's fake API, policy and environment.
func wire(t *testing.T, rg *rig) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	was := Source{Now: Default.Now, Getenv: Default.Getenv, Policy: Default.Policy, NewAPI: Default.NewAPI}
	Default.Now, Default.Getenv, Default.Policy, Default.NewAPI = rg.src.Now, rg.src.Getenv, rg.src.Policy, rg.src.NewAPI
	Default.pins = pinTable{}
	t.Cleanup(func() {
		Default.Now, Default.Getenv, Default.Policy, Default.NewAPI = was.Now, was.Getenv, was.Policy, was.NewAPI
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

func (i *intakeRig) post(d delivery, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	i.h.ServeHTTP(w, d.request(body))
	return w
}

// postTampered signs body and sends sent in its place.
func (i *intakeRig) postTampered(d delivery, body, sent []byte) *httptest.ResponseRecorder {
	req := d.request(body)
	req.Body = io.NopCloser(bytes.NewReader(sent))
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

// TestIntakeEndToEnd proves acceptance lines 1 and 3 through NODE-10's intake: a fork merge request
// signed with the project's own secret is enqueued without an allowlist entry and its stored facts
// reach a classifier through providers.RefetchFacts (the "gitlab" provider's seam) with the fork
// facts read from the API; a bare-token delivery stays under --allowed-repos; a tampered body, a
// stale timestamp and a wrong token are 403 with nothing enqueued.
func TestIntakeEndToEnd(t *testing.T) {
	rg := newRig(t, both())
	rg.gl.set("/projects/acme%2Fapp/merge_requests/7", 200, `{"iid":7,"sha":"`+shaD+`","source_project_id":2002,"target_project_id":1001}`)
	wire(t, rg)
	in := newIntake(t, "acme/tokened")
	mr := fixture(t, "mr_fork_open")
	signed := func(id string, body []byte) *httptest.ResponseRecorder {
		return in.post(delivery{event: "Merge Request Hook", id: id, ts: rg.now, signSecret: signingSecret}, body)
	}

	w := signed("msg-fork", mr)
	if w.Code != http.StatusAccepted {
		t.Fatalf("fork MR with its own secret = %d %s", w.Code, w.Body)
	}
	stored := in.facts(w)
	if stored.Auth != "per-repo" || stored.Repo != "acme/app" || stored.Kind != "pull_request" || stored.DeliveryID != "msg-fork" {
		t.Fatalf("stored facts = %+v", stored)
	}
	fresh, err := providers.RefetchFacts(context.Background(), stored) // TRUST-13's hook
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Fork.Value == nil || !*fresh.Fork.Value || fresh.Fork.Source != "api" || fresh.HeadRepo.Value == nil || *fresh.HeadRepo.Value != "contrib/app-fork" ||
		fresh.ActorPermission.Value == nil || *fresh.ActorPermission.Value != "write" || fresh.ProtectedRef.Value == nil || !*fresh.ProtectedRef.Value {
		t.Fatalf("the classifier must receive the fork facts read from the API: %+v", fresh)
	}

	tokened := replace(fixture(t, "push"), `"path_with_namespace":"acme/app"`, `"path_with_namespace":"acme/tokened"`)
	bearer := delivery{event: "Push Hook", uuid: "uuid-b", token: bareToken}
	if w = in.post(bearer, tokened); w.Code != http.StatusAccepted || in.facts(w).Auth != "bearer" {
		t.Fatalf("allowlisted bearer delivery = %d %s", w.Code, w.Body)
	}
	before := in.queued()
	notListed := replace(tokened, `"acme/tokened"`, `"acme/unlisted"`)
	if w = in.post(bearer, notListed); w.Code != http.StatusForbidden || in.queued() != before {
		t.Fatalf("a project with no secret = %d", w.Code)
	}
	// the token project is not in --allowed-repos here: the bearer class is held to the allowlist
	in2 := newIntake(t, "someone/else")
	if w = in2.post(bearer, tokened); w.Code != http.StatusForbidden || in2.queued() != 0 {
		t.Fatalf("bearer delivery outside --allowed-repos = %d queued %d", w.Code, in2.queued())
	}
	// 403 for each failure of acceptance line 1, nothing enqueued
	tampered := delivery{event: "Merge Request Hook", id: "msg-t", ts: rg.now, signSecret: signingSecret}
	old := delivery{event: "Merge Request Hook", id: "msg-old", ts: rg.now.Add(-6 * time.Minute), signSecret: signingSecret}
	cases := map[string]*httptest.ResponseRecorder{
		"tampered body":     in.postTampered(tampered, mr, replace(mr, `"username":"dev1"`, `"username":"evil"`)),
		"timestamp 6 min":   in.post(old, mr),
		"wrong token":       in.post(delivery{event: "Push Hook", uuid: "u", token: "synthetic-wrong-token"}, tokened),
		"signing + token":   in.post(delivery{event: "Merge Request Hook", uuid: "u", token: bareToken}, mr),
		"no credentials":    in.post(delivery{event: "Merge Request Hook"}, mr),
		"signature of nada": in.post(delivery{event: "Merge Request Hook", id: "x", ts: rg.now, signature: "v1," + strconv.Itoa(0)}, mr),
	}
	for name, w := range cases {
		if w.Code != http.StatusForbidden || in.queued() != before {
			t.Fatalf("%s = %d (queued %d -> %d)", name, w.Code, before, in.queued())
		}
	}
	// a verified delivery that must not run is 200 accepted:false
	closed := replace(mr, `"action":"open"`, `"action":"close"`)
	if w = signed("msg-closed", closed); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"accepted":false`) || in.queued() != before {
		t.Fatalf("closed MR = %d %s", w.Code, w.Body)
	}
}
