package hosted_test

// Purpose: Collect, Liveness, Cancel and the whole driver path of P7-HOST-03 against fakeGH.
// Inputs:  fakeGH, the pinned workflow rendering, evidence zips.
// Outputs: pass or fail.
// Constraints: no live network; each test's doc comment cites the acceptance line it proves.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// dispatched runs one attempt to completion on f and returns the provider, the handle and the attempt.
func dispatched(t *testing.T, f *fakeGH, mut func(*model.Evidence)) (*hosted.Provider, providers.Handle) {
	t.Helper()
	v15(t)
	f.Workflow = pinnedWorkflow(t)
	p := f.provider(options{})
	h, err := p.Dispatch(context.Background(), attemptFor("att-1", 2))
	if err != nil {
		t.Fatal(err)
	}
	f.finish(9001, "att-1", testRevision, mut)
	return p, h
}

// TestCollectRejections proves the evidence acceptance of Epic D5 through the real Collect path: each
// refusal is an E705 with its own reason key and nothing is accepted.
func TestCollectRejections(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		mut          func(*fakeGH)
	}{
		{"event is not workflow_dispatch", hostedev.ReasonEvent, func(f *fakeGH) { f.Runs[9001].Event = "push" }},
		{"another workflow path", hostedev.ReasonPath, func(f *fakeGH) { f.Runs[9001].Path = ".github/workflows/other.yml" }},
		{"another dispatch ref", hostedev.ReasonRef, func(f *fakeGH) { f.Runs[9001].Branch = "feature" }},
		{"nonce missing from the title", hostedev.ReasonNonce, func(f *fakeGH) { f.Runs[9001].Title = "nself-ci other" }},
		{"edited workflow at head_sha", hostedev.ReasonEdited, func(f *fakeGH) {
			f.Runs[9001].WorkflowOverride = append(pinnedWorkflow(t), ' ')
		}},
		{"an unpinned agent commit", hostedev.ReasonEdited, func(f *fakeGH) {
			f.Runs[9001].WorkflowOverride = []byte(strings.ReplaceAll(string(pinnedWorkflow(t)), testCommit, strings.Repeat("e", 40)))
		}},
		{"stale template", hostedev.ReasonStale, func(f *fakeGH) {
			f.Runs[9001].WorkflowOverride = []byte(strings.Replace(string(pinnedWorkflow(t)), "# template: v1", "# template: v0", 1))
		}},
		{"missing artifact", hostedev.ReasonMissing, func(f *fakeGH) { f.Runs[9001].ArtifactID = 0 }},
		{"17 MiB artifact", hostedev.ReasonTooLarge, func(f *fakeGH) { f.Runs[9001].ArtifactSizeReport = 17 << 20 }},
		{"re-run of a run", hostedev.ReasonRerun, func(f *fakeGH) { f.Runs[9001].RunAttempt = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGH(t)
			p, h := dispatched(t, f, nil)
			tc.mut(f)
			_, err := p.Collect(context.Background(), h)
			wantCode(t, err, "E705", tc.reason)
		})
	}
	t.Run("evidence for another attempt", func(t *testing.T) {
		f := newFakeGH(t)
		p, h := dispatched(t, f, func(e *model.Evidence) { e.Attempt.ID = "att-other" })
		_, err := p.Collect(context.Background(), h)
		wantCode(t, err, "E705", hostedev.ReasonAttempt)
	})
	t.Run("evidence for another revision", func(t *testing.T) {
		f := newFakeGH(t)
		p, h := dispatched(t, f, func(e *model.Evidence) { e.Revision = strings.Repeat("9", 40) })
		_, err := p.Collect(context.Background(), h)
		wantCode(t, err, "E705", hostedev.ReasonRevision)
	})
	t.Run("a handle this coordinator never journalled", func(t *testing.T) {
		f := newFakeGH(t)
		_, h := dispatched(t, f, nil)
		other := f.provider(options{}) // a fresh journal
		_, err := other.Collect(context.Background(), h)
		wantCode(t, err, "E705", hosted.ReasonIntentUnknown)
	})
}

// TestLivenessAndCancel proves the Epic D5 state mapping, 404 as vanished and an idempotent cancel (409 is success).
func TestLivenessAndCancel(t *testing.T) {
	v15(t)
	f := newFakeGH(t)
	p := f.provider(options{})
	ctx := context.Background()
	h, err := p.Dispatch(ctx, attemptFor("att-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	for status, want := range map[string]providers.LiveState{"queued": providers.Queued, "in_progress": providers.Running, "waiting": providers.Running, "completed": providers.Finished} {
		f.Runs[9001].Status = status
		if got, err := p.Liveness(ctx, h); err != nil || got != want {
			t.Errorf("%s -> %v %v", status, got, err)
		}
	}
	f.Runs[9001].Status = "mystery"
	if _, err = p.Liveness(ctx, h); err == nil {
		t.Error("an unknown status was accepted")
	}
	f.Runs[9001].Status = "queued"
	if err = p.Cancel(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err = p.Cancel(ctx, h); err != nil { // the second one meets 409
		t.Fatalf("second cancel: %v", err)
	}
	delete(f.Runs, 9001)
	if got, err := p.Liveness(ctx, h); err != nil || got != providers.Vanished {
		t.Errorf("vanished -> %v %v", got, err)
	}
	if err = p.Cancel(ctx, h); err != nil {
		t.Errorf("cancel of a gone run: %v", err)
	}
	bad := providers.Handle{Provider: hosted.ProviderID, RunID: "not-a-run"}
	if _, err = p.Liveness(ctx, bad); err == nil {
		t.Error("a malformed handle was accepted")
	}
}

type resultSink struct {
	transport.Sink
	mu      sync.Mutex
	results []protocol.Result
}

func (s *resultSink) Result(_ context.Context, r protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	return nil
}

var registerE2E sync.Once

// TestAcceptedEvidence proves accepted evidence is stored with producer.kind self-reported,
// signature.kind coordinator-recorded and every provider_attestation field set: a full hosted attempt
// runs through the transport driver of kind "provider" and the provider's Collect.
func TestAcceptedEvidence(t *testing.T) {
	v15(t)
	f := newFakeGH(t)
	f.Workflow = pinnedWorkflow(t)
	p := f.provider(options{ID: "github-hosted-e2e"})
	registerE2E.Do(func() {
		if err := providers.Register(p); err != nil {
			t.Fatal(err)
		}
	})
	if cur, _ := providers.Lookup("github-hosted-e2e"); cur != providers.Provider(p) {
		t.Skip("the e2e provider belongs to an earlier run of this test binary")
	}
	f.Extra = func(r *fakeRun) {
		r.Status, r.ArtifactID, r.ArtifactName = "completed", 4000+r.ID, hosted.EvidenceArtifact(r.Nonce)
		r.EvidenceZip = evidenceZip(t, "att-1", testRevision, nil)
	}
	d := runtime.Default()
	d.SetBook(runtime.NewMemBook())
	d.SetClock(func(context.Context, time.Duration) error { return nil }, nil)
	d.SetFacts(func(ctx context.Context, node model.Capability, l protocol.Lease) (runtime.AttemptFacts, error) {
		facts, err := hosted.Facts(ctx, node, l)
		facts.Trust = "owner"
		return facts, err
	})
	t.Cleanup(func() { d.SetBook(runtime.NewMemBook()); d.SetFacts(hosted.Facts); d.SetClock(nil, nil) })
	caps, err := p.Describe(context.Background(), providers.Query{Project: "widgets", Repo: "acme/widgets"})
	if err != nil || len(caps) == 0 {
		t.Fatalf("describe: %v", err)
	}
	node := caps[0]
	node.Identity.Provider = "github-hosted-e2e"
	lease := protocol.Lease{LeaseID: "lease-1", AttemptID: "att-1", Epoch: 1, CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "20m"}, Checkout: protocol.Checkout{Revision: testRevision}}
	sink := &resultSink{}
	if err = d.Run(context.Background(), node, lease, sink); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(sink.results) != 1 || len(f.Runs) != 1 {
		t.Fatalf("results=%d runs=%d", len(sink.results), len(f.Runs))
	}
	var ev model.Evidence
	if err = json.Unmarshal(sink.results[0].Evidence, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Producer.Kind != "self-reported" || ev.Signature.Kind != "coordinator-recorded" || ev.ProviderAttestation == nil {
		t.Fatalf("evidence: producer=%v signature=%v attestation=%v", ev.Producer, ev.Signature, ev.ProviderAttestation)
	}
	att := reflect.ValueOf(*ev.ProviderAttestation)
	for i := 0; i < att.NumField(); i++ {
		if att.Field(i).IsZero() {
			t.Errorf("provider_attestation.%s is empty", att.Type().Field(i).Name)
		}
	}
	if ev.ProviderAttestation.Provider != "github-hosted-e2e" || ev.ProviderAttestation.RunID != "acme/widgets/9001" || ev.ProviderAttestation.TemplateVersion != "v1" {
		t.Fatalf("attestation: %+v", *ev.ProviderAttestation)
	}
}

// TestStoreJournal proves the durable journal: a restart sees the same nonce for the same attempt
// number, a higher number replaces it and reports the earlier record, a lower number is refused, and
// a record is found by nonce for Collect.
func TestStoreJournal(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	j := hosted.NewStoreJournal(st)
	ctx := context.Background()
	rec := func(n int, nonce string) hosted.Record {
		return hosted.Record{Attempt: "att-1", N: n, Revision: testRevision, SpecDigest: "sha256:" + strings.Repeat("a", 64), Nonce: nonce, Ref: "main"}
	}
	first, prev, err := j.Begin(ctx, rec(1, "n1"))
	if err != nil || prev != nil || first.Nonce != "n1" {
		t.Fatalf("first: %+v %v %v", first, prev, err)
	}
	again, prev, err := j.Begin(ctx, rec(1, "other"))
	if err != nil || prev != nil || again.Nonce != "n1" {
		t.Fatalf("again: %+v %v %v", again, prev, err)
	}
	next, prev, err := j.Begin(ctx, rec(2, "n2"))
	if err != nil || next.Nonce != "n2" || prev == nil || prev.Nonce != "n1" {
		t.Fatalf("next: %+v %+v %v", next, prev, err)
	}
	if _, _, err = j.Begin(ctx, rec(1, "n3")); err == nil {
		t.Fatal("a lower attempt number was accepted")
	}
	got, ok, err := j.ByNonce(ctx, "n1")
	if err != nil || !ok || got.N != 1 || got.Revision != testRevision {
		t.Fatalf("by nonce: %+v %v %v", got, ok, err)
	}
	if _, ok, _ = j.ByNonce(ctx, "nope"); ok {
		t.Fatal("an unknown nonce was found")
	}
}
