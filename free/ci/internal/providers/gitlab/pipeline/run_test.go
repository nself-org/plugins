package pipeline_test

// Purpose: Liveness, Cancel and Describe tests of P7-HOST-06, including TestNoPublicLabel.
// Inputs:  the fakeGL routes.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
)

// TestLivenessMapping proves the status table: every documented pipeline status maps to queued,
// running or finished, a deleted pipeline is vanished and an unknown status is a retryable E704.
func TestLivenessMapping(t *testing.T) {
	ctx := context.Background()
	f, p, h := dispatched(t, "18.2.1-ee", nil)
	for status, want := range map[string]providers.LiveState{
		"created": providers.Queued, "waiting_for_resource": providers.Queued, "preparing": providers.Queued, "waiting_for_callback": providers.Queued,
		"pending": providers.Queued, "scheduled": providers.Queued, "manual": providers.Queued,
		"running": providers.Running, "canceling": providers.Running,
		"success": providers.Finished, "failed": providers.Finished, "canceled": providers.Finished, "skipped": providers.Finished,
	} {
		f.Pipes[9001].Status = status
		got, err := p.Liveness(ctx, h)
		if err != nil || got != want {
			t.Errorf("%s -> %q %v, want %q", status, got, err, want)
		}
	}
	f.Pipes[9001].Status = "from_the_future"
	_, err := p.Liveness(ctx, h)
	wantCode(t, err, "E704", "run.status")
	delete(f.Pipes, 9001)
	if got, err := p.Liveness(ctx, h); err != nil || got != providers.Vanished {
		t.Errorf("deleted pipeline: %q %v", got, err)
	}
	if _, err := p.Liveness(ctx, providers.Handle{Provider: "gitlab", RunID: "acme/widgets/zero"}); err == nil {
		t.Error("a bad handle was accepted")
	}
}

// TestCancelIsIdempotent proves Cancel twice, on a finished pipeline and on a deleted one never fails.
func TestCancelIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f, p, h := dispatched(t, "18.2.1-ee", nil)
	for i := 0; i < 2; i++ {
		if err := p.Cancel(ctx, h); err != nil {
			t.Fatalf("cancel %d: %v", i, err)
		}
	}
	f.finish(9001, "attempt-1", testRevision, nil)
	if err := p.Cancel(ctx, h); err != nil || f.Pipes[9001].Status != "success" {
		t.Fatalf("finished pipeline: %v %s", err, f.Pipes[9001].Status)
	}
	delete(f.Pipes, 9001)
	if err := p.Cancel(ctx, h); err != nil {
		t.Fatalf("deleted pipeline: %v", err)
	}
}

// TestNoPublicLabel proves acceptance line 7: Describe never emits nself.visibility=public, for a public
// project, a private one and an internal one, because gitlab.com meters public projects too; it emits
// one capability per allowed shared-runner tag, none of a larger runner, with the economics class
// included and the allowance left unknown.
func TestNoPublicLabel(t *testing.T) {
	v15(t)
	for _, visibility := range []string{"public", "private", "internal"} {
		f := newFakeGL(t)
		f.Visibility = visibility
		caps, err := f.provider(options{}).Describe(context.Background(), providers.Query{Project: "widgets", Repo: "acme/widgets"})
		if err != nil || len(caps) != 2 {
			t.Fatalf("%s: %d capabilities, %v", visibility, len(caps), err)
		}
		for _, c := range caps {
			for _, l := range c.Identity.Labels {
				if strings.Contains(l, "visibility") || strings.Contains(l, "public") {
					t.Errorf("%s: label %q", visibility, l)
				}
			}
			if c.Identity.Provider != "gitlab" || c.Identity.Transport != "provider" || *c.Economics.Class.Value != "included" ||
				strings.Contains(c.Identity.ID, "large") || *c.Trust.Isolation.Value != "hosted-disposable" {
				t.Errorf("%s: capability %+v", visibility, c.Identity)
			}
			if c.Economics.Class.Confidence != "known" {
				t.Errorf("economics class confidence %q", c.Economics.Class.Confidence)
			}
			if err := model.ValidateCapability(c); err != nil {
				t.Errorf("%s: capability invalid: %v", visibility, err)
			}
		}
		if len(f.Reqs) != 1 {
			t.Errorf("%s: Describe made %d requests, want the one project lookup", visibility, len(f.Reqs))
		}
	}
}

// TestDescribeEdges proves an empty query describes nothing without a request, a bad path is E702,
// an unreadable project is the funnel's error (the contributor then skips the provider), a missing
// credential is E700, and v1.4 describes nothing.
func TestDescribeEdges(t *testing.T) {
	v15(t)
	ctx := context.Background()
	f := newFakeGL(t)
	p := f.provider(options{})
	if caps, err := p.Describe(ctx, providers.Query{}); err != nil || caps != nil || len(f.Reqs) != 0 {
		t.Errorf("empty query: %v %v %d", caps, err, len(f.Reqs))
	}
	_, err := p.Describe(ctx, providers.Query{Repo: "widgets"})
	wantCode(t, err, "E702", "repo")
	f.ProjectStatus = 404
	if _, err = p.Describe(ctx, providers.Query{Repo: "acme/widgets"}); err == nil {
		t.Error("an unreadable project described capacity")
	}
	f.ProjectStatus = 401
	_, err = p.Describe(ctx, providers.Query{Repo: "acme/widgets"})
	wantCode(t, err, "E701", "")
	t.Setenv("NSELF_V15", "")
	if caps, err := p.Describe(ctx, providers.Query{Repo: "acme/widgets"}); err != nil || caps != nil {
		t.Errorf("v1.4: %v %v", caps, err)
	}
}

// TestRegistration proves the provider registers as "gitlab" (the only value the runner-capability
// schema allows), the lease profile is the hosted one, and the trust facts seam delegates: without an
// installed refetcher the facts come back unchanged, with one they come from it.
func TestRegistration(t *testing.T) {
	if errs := pipeline.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	got, ok := providers.Lookup("gitlab")
	if !ok || got.ID() != "gitlab" {
		t.Fatalf("lookup: %v %v", got, ok)
	}
	d, ok := got.(providers.Dispatcher)
	if !ok || d.Profile().TTL <= 0 || d.Profile().Poll <= 0 {
		t.Errorf("profile: %+v", d)
	}
	in := model.TriggerFacts{Source: "gitlab"}
	same, err := providers.RefetchFacts(context.Background(), in)
	if err != nil || same.Source != "gitlab" {
		t.Fatalf("no refetcher: %+v %v", same, err)
	}
	p := pipeline.New(pipeline.Config{})
	p.UseTrustFacts(fakeRefetch{})
	if _, err = p.Refetch(context.Background(), in); err == nil || err.Error() != "refetched" {
		t.Errorf("the installed refetcher was not used: %v", err)
	}
	p.UseTrustFacts(nil)
	if out, err := p.Refetch(context.Background(), in); err != nil || out.Source != "gitlab" {
		t.Errorf("removed refetcher: %+v %v", out, err)
	}
}

type fakeRefetch struct{}

func (fakeRefetch) Refetch(context.Context, model.TriggerFacts) (model.TriggerFacts, error) {
	return model.TriggerFacts{}, errors.New("refetched")
}
