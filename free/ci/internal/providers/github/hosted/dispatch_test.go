package hosted_test

// Purpose: the dispatch acceptance tests of P7-HOST-03 (idempotent dispatch, untrusted ref, refusals).
// Inputs:  fakeGH (helpers_test.go).
// Outputs: pass or fail.
// Constraints: no live network; each test's doc comment cites the acceptance line it proves.

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
)

func v15(t *testing.T) { t.Helper(); t.Setenv("NSELF_V15", "1") }

func wantCode(t *testing.T, err error, code, reason string) {
	t.Helper()
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != code || (reason != "" && pe.Reason != reason) {
		t.Fatalf("want %s/%s, got %v", code, reason, err)
	}
}

// TestIdempotentDispatch proves the Epic D5 / review F10 acceptance: a request that times out after
// GitHub created the run leads to adoption of that run (one run total); a retry attempt cancels the
// previous attempt's run; two runs with one nonce, or one by another actor, are E705 run.ambiguous;
// the 200 path (workflow_run_id, body carries return_run_details true) and the 204 listing path both work.
func TestIdempotentDispatch(t *testing.T) {
	v15(t)
	ctx := context.Background()
	t.Run("200 path", func(t *testing.T) {
		f := newFakeGH(t)
		h, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil {
			t.Fatal(err)
		}
		posts := f.requests("POST", "/dispatches")
		if len(posts) != 1 || len(f.Runs) != 1 || h.RunID != "acme/widgets/9001" || h.Provider != hosted.ProviderID || h.Ref != "main" {
			t.Fatalf("posts=%d runs=%d handle=%+v", len(posts), len(f.Runs), h)
		}
		ref, in, body := inputsOf(t, posts[0])
		if body["return_run_details"] != true || ref != "main" || in["revision"] != testRevision || in["attempt"] != "att-1" ||
			in["runner_label"] != "ubuntu-24.04" || in["compat_mode"] != "v1.5" || in["timeout_minutes"] != "30" || in["nonce"] != h.Nonce {
			t.Fatalf("body: %v", body)
		}
		if len(f.requests("GET", "/runs")) != 0 {
			t.Fatal("the 200 path listed runs")
		}
	})
	t.Run("204 path finds the run by nonce and actor", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "204"
		h, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil || h.RunID != "acme/widgets/9001" {
			t.Fatalf("%+v %v", h, err)
		}
		lists := f.requests("GET", "/workflows/nself-ci-dispatch.yml/runs")
		if len(lists) != 1 || !strings.Contains(lists[0].Query, "event=workflow_dispatch") || !strings.Contains(lists[0].Query, "created=2026-10-19T11%3A58%3A00Z..2026-10-19T12%3A02%3A00Z") {
			t.Fatalf("listing: %+v", lists)
		}
	})
	t.Run("timeout after the run was created adopts it", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "create-then-502"
		h, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil || h.RunID != "acme/widgets/9001" {
			t.Fatalf("%+v %v", h, err)
		}
		if n := len(f.requests("POST", "/dispatches")); n != 1 || len(f.Runs) != 1 {
			t.Fatalf("posts=%d runs=%d: the dispatch was resent", n, len(f.Runs))
		}
	})
	t.Run("no run appears: the transient error is returned and nothing is resent", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "nothing-then-502"
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		wantCode(t, err, "E704", "")
		if n := len(f.requests("POST", "/dispatches")); n != 1 {
			t.Fatalf("posts=%d", n)
		}
	})
	t.Run("a retry attempt cancels the previous attempt's run", func(t *testing.T) {
		f := newFakeGH(t)
		p := f.provider(options{})
		first, err := p.Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil {
			t.Fatal(err)
		}
		second, err := p.Dispatch(ctx, attemptFor("att-1", 2))
		if err != nil || second.RunID == first.RunID || second.Nonce == first.Nonce {
			t.Fatalf("%+v %v", second, err)
		}
		cancels := f.requests("POST", "/runs/9001/cancel")
		if len(cancels) != 1 || !f.Runs[9001].Cancelled || f.Runs[9002].Cancelled {
			t.Fatalf("cancels=%d first=%v second=%v", len(cancels), f.Runs[9001].Cancelled, f.Runs[9002].Cancelled)
		}
		_, err = p.Dispatch(ctx, attemptFor("att-1", 1))
		wantCode(t, err, "E708", "attempt.superseded")
	})
	t.Run("the same attempt number after a restart adopts its run", func(t *testing.T) {
		f := newFakeGH(t)
		journal := hosted.NewMemJournal()
		h1, err := f.provider(options{Journal: journal}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil {
			t.Fatal(err)
		}
		h2, err := f.provider(options{Journal: journal, Nonce: func() (string, error) { return strings.Repeat("f", 32), nil }}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil || h2 != h1 || len(f.Runs) != 1 || len(f.requests("POST", "/dispatches")) != 1 {
			t.Fatalf("%+v %v runs=%d", h2, err, len(f.Runs))
		}
	})
	t.Run("the run on the second page of a busy listing is found", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "204"
		for i := 0; i < 100; i++ { // one full page of other people's runs, older than ours
			f.NextID++
			f.Runs[f.NextID] = &fakeRun{ID: f.NextID, Status: "completed", Event: "workflow_dispatch", Title: "nself-ci other", Login: testLogin}
		}
		h, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		if err != nil || h.RunID != "acme/widgets/9101" {
			t.Fatalf("%+v %v", h, err)
		}
		if pages := len(f.requests("GET", "/workflows/nself-ci-dispatch.yml/runs")); pages != 2 {
			t.Fatalf("listing pages = %d", pages)
		}
	})
	t.Run("two runs with one nonce are ambiguous", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "204"
		f.Extra = func(r *fakeRun) { f.NextID++; dup := *r; dup.ID = f.NextID; f.Runs[dup.ID] = &dup }
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		wantCode(t, err, "E705", hostedev.ReasonAmbiguous)
	})
	t.Run("a run by another actor is ambiguous", func(t *testing.T) {
		f := newFakeGH(t)
		f.DispatchMode = "204"
		f.Extra = func(r *fakeRun) { r.Login = "someone-else" }
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		wantCode(t, err, "E705", hostedev.ReasonAmbiguous)
	})
}

// TestUntrustedRef proves the Epic D5 acceptance: an untrusted attempt dispatches with ref
// providers.github.untrusted_ref, a trusted one with the default branch, both send the admitted
// revision, and no refs/ lookup request occurs.
func TestUntrustedRef(t *testing.T) {
	v15(t)
	ctx := context.Background()
	set := hosted.Settings{UntrustedRef: "ci/custom-untrusted", AgentSource: hosted.AgentSource}
	for _, tc := range []struct {
		trust, ref string
		lookups    int
	}{{"untrusted", "ci/custom-untrusted", 0}, {"", "ci/custom-untrusted", 0}, {"stranger", "ci/custom-untrusted", 0}, {"owner", "trunk", 1}, {"internal", "trunk", 1}, {"collaborator", "trunk", 1}} {
		t.Run("trust "+tc.trust, func(t *testing.T) {
			f := newFakeGH(t)
			f.DefaultBranch = "trunk"
			a := attemptFor("att-1", 1)
			a.Trust = tc.trust
			h, err := f.provider(options{Settings: set}).Dispatch(ctx, a)
			if err != nil {
				t.Fatal(err)
			}
			ref, in, _ := inputsOf(t, f.requests("POST", "/dispatches")[0])
			if ref != tc.ref || h.Ref != tc.ref || in["revision"] != testRevision {
				t.Fatalf("ref=%q handle=%+v revision=%v", ref, h, in["revision"])
			}
			if n := len(f.requests("GET", "/repos/acme/widgets")); n != 0 && n != tc.lookups {
				t.Fatalf("lookups=%d", n)
			}
			for _, r := range f.Reqs {
				if strings.Contains(r.Path, "/git/") || strings.Contains(r.Path, "/refs") || strings.Contains(r.Path, "/commits") || strings.Contains(r.Path, "/branches") {
					t.Fatalf("a ref was resolved: %s", r.Path)
				}
			}
		})
	}
	t.Run("a fork pull request head travels as fetch_ref", func(t *testing.T) {
		f := newFakeGH(t)
		a := attemptFor("att-1", 1)
		a.Trust, a.FetchRef = "untrusted", "refs/pull/12/head"
		if _, err := f.provider(options{}).Dispatch(ctx, a); err != nil {
			t.Fatal(err)
		}
		if _, in, _ := inputsOf(t, f.requests("POST", "/dispatches")[0]); in["fetch_ref"] != "refs/pull/12/head" {
			t.Fatalf("inputs: %v", in)
		}
	})
}

// TestDispatchRefusals proves the coordinator only sends values the workflow's choice
// inputs accept: a label outside labels.yaml, a larger runner, an unknown compat mode and a timeout
// above 360 minutes are E708 before any request, and a timeout rounds up to the next option.
func TestDispatchRefusals(t *testing.T) {
	v15(t)
	ctx := context.Background()
	big := make([]byte, 80<<10)
	_, _ = rand.New(rand.NewSource(1)).Read(big)
	for _, tc := range []struct {
		name, reason string
		mut          func(*providers.Attempt)
	}{
		{"free-text label", "label.not_allowed", func(a *providers.Attempt) { a.Label = "my-own-runner" }},
		{"larger runner", "label.not_allowed", func(a *providers.Attempt) { a.Label = "ubuntu-latest-large" }},
		{"compat mode", "compat_mode", func(a *providers.Attempt) { a.CompatMode = "v9" }},
		{"timeout above 360", "timeout.over_360", func(a *providers.Attempt) { a.TimeoutMs = 361 * 60 * 1000 }},
		{"spec too large", "spec.too_large", func(a *providers.Attempt) { a.Spec = big }},
		{"spec digest", "spec.digest", func(a *providers.Attempt) { a.SpecDigest = "sha256:" + strings.Repeat("0", 64) }},
		{"repo", "repo", func(a *providers.Attempt) { a.Repo = "no-slash" }},
		{"revision", "revision", func(a *providers.Attempt) { a.Revision = "bad rev" }},
		{"fetch ref", "fetch_ref", func(a *providers.Attempt) { a.FetchRef = "refs/heads/main" }},
		{"no attempt id", "attempt", func(a *providers.Attempt) { a.ID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGH(t)
			a := attemptFor("att-1", 1)
			tc.mut(&a)
			_, err := f.provider(options{}).Dispatch(ctx, a)
			wantCode(t, err, "E708", tc.reason)
			if len(f.Reqs) != 0 {
				t.Fatalf("a request was made before the refusal: %v", f.Reqs)
			}
		})
	}
	for ms, want := range map[int64]string{0: "60", 1: "10", 10 * 60000: "10", 10*60000 + 1: "30", 61 * 60000: "120", 360 * 60000: "360"} {
		f := newFakeGH(t)
		a := attemptFor("att-1", 1)
		a.TimeoutMs = ms
		if _, err := f.provider(options{}).Dispatch(ctx, a); err != nil {
			t.Fatal(err)
		}
		if _, in, _ := inputsOf(t, f.requests("POST", "/dispatches")[0]); in["timeout_minutes"] != want {
			t.Errorf("timeout %d ms -> %v, want %s", ms, in["timeout_minutes"], want)
		}
	}
	t.Run("without v1.5 nothing is dispatched", func(t *testing.T) {
		t.Setenv("NSELF_V15", "")
		f := newFakeGH(t)
		_, err := f.provider(options{}).Dispatch(ctx, attemptFor("att-1", 1))
		wantCode(t, err, "E708", "")
		if len(f.Reqs) != 0 {
			t.Fatal("a request was made")
		}
	})
}
