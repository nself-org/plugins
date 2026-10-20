package webhook

// Purpose: normalization tests (P7-HOST-21 acceptance line 2): goldens for Push, Tag Push, Merge
// Request (open, reopen, update with oldrev) and Job Hook (pending with tag nself, a runner demand), and
// the declines for everything else.
// Inputs:  the recorded deliveries (testdata/*.json) and the fake API (testdata/api/routes.json).
// Outputs: goldens under testdata/golden; pass or fail.
// Constraints: rewrite goldens with `go test -run TestNormalizeGoldens -update`, then read the diff.

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// wantDecline fails unless err is a *Decline whose E706 reason is key and whose text contains text.
func wantDecline(t *testing.T, what string, err error, key, text string) {
	t.Helper()
	var d *Decline
	if !errors.As(err, &d) {
		t.Fatalf("%s: err = %v, want a decline", what, err)
	}
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E706" || pe.Reason != key || pe.Status != http.StatusOK {
		t.Fatalf("%s: unwrapped error = %+v, want E706 %s with status 200", what, pe, key)
	}
	if !strings.Contains(d.DeclineReason(), text) {
		t.Fatalf("%s: reason %q lacks %q", what, d.DeclineReason(), text)
	}
}

// TestNormalizeGoldens proves acceptance line 2: Push, Tag Push, Merge Request (open, reopen, update
// with oldrev) and Job Hook pending with the runner tag nself normalize into trigger facts (goldens);
// other merge request actions, an update without new commits, a closed merge request, deletes, untagged
// and running jobs and unserved events are dropped with 200 accepted:false (a *Decline).
func TestNormalizeGoldens(t *testing.T) {
	events := map[string]string{
		"push": "Push Hook", "push_new_branch": "Push Hook", "tag_push": "Tag Push Hook", "tag_push_nosha": "Tag Push Hook",
		"mr_open": "Merge Request Hook", "mr_reopen": "Merge Request Hook", "mr_update_oldrev": "Merge Request Hook",
		"mr_fork_open": "Merge Request Hook", "job_pending_nself": "Job Hook",
	}
	for name, event := range events {
		t.Run(name, func(t *testing.T) {
			rg := newRig(t, both())
			f, err := rg.signed(event, fixture(t, name))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if verr := f.Validate(); verr != nil {
				t.Fatalf("%s: facts do not validate: %v", name, verr)
			}
			golden(t, name, f)
		})
	}

	t.Run("key facts", func(t *testing.T) {
		rg := newRig(t, both())
		f, _ := rg.signed("Merge Request Hook", fixture(t, "mr_fork_open"))
		if f.Fork.Value == nil || !*f.Fork.Value || f.Fork.Source != "payload" || f.HeadRepo.Value == nil || *f.HeadRepo.Value != "contrib/app-fork" {
			t.Fatalf("fork MR facts: %+v %+v", f.Fork, f.HeadRepo)
		}
		if f.Ref != "refs/merge-requests/7/head" || f.BaseRef != "refs/heads/main" || f.PRNumber != 7 || f.Revision != shaD {
			t.Fatalf("MR names the merge request, not the source branch: %+v", f)
		}
		f, _ = rg.signed("Job Hook", fixture(t, "job_pending_nself"))
		if f.Kind != "runner_demand" || f.Ref != "" || f.RunnerLabels.Value == nil || len(*f.RunnerLabels.Value) != 2 {
			t.Fatalf("job demand facts: %+v", f)
		}
		f, _ = rg.signed("Push Hook", fixture(t, "push"))
		if f.Visibility.Value == nil || *f.Visibility.Value != "private" || f.Visibility.Source != "payload" {
			t.Fatalf("visibility: %+v", f.Visibility)
		}
	})

	declines := []struct {
		name, fixture, event, key, text string
	}{
		{"deleted branch", "push_delete", "Push Hook", reasonUnsupported, "delete push"},
		{"push to a tag ref", "push_to_tag_ref", "Push Hook", reasonUnsupported, "refs/tags/v1.0.0"},
		{"deleted tag", "tag_push_delete", "Tag Push Hook", reasonUnsupported, "delete push"},
		{"mr update without new commits", "mr_update_title", "Merge Request Hook", reasonUnsupported, "without new commits"},
		{"mr closed", "mr_close", "Merge Request Hook", reasonUnsupported, `mr action "close"`},
		{"mr approved", "mr_approved", "Merge Request Hook", reasonUnsupported, `mr action "approved"`},
		{"mr into another project", "mr_other_target", "Merge Request Hook", reasonUnsupported, "target project differs"},
		{"job without the tag", "job_pending_untagged", "Job Hook", reasonUnsupported, "runner tag nself"},
		{"job already running", "job_running", "Job Hook", reasonUnsupported, `job status "running"`},
		{"unserved event", "push", "Pipeline Hook", reasonUnsupported, `event "Pipeline Hook"`},
		{"event header differs from the body", "push", "Tag Push Hook", reasonUnsupported, "does not match"},
	}
	for _, c := range declines {
		t.Run(c.name, func(t *testing.T) {
			rg := newRig(t, both())
			_, err := rg.signed(c.event, fixture(t, c.fixture))
			wantDecline(t, c.name, err, c.key, c.text)
		})
	}

	t.Run("a body that is not a payload", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.signed("Push Hook", []byte(`{"project":{"id":1001,"path_with_namespace":"acme/app"},"after":7}`))
		wantDecline(t, "after is a number", err, reasonUnsupported, "supported GitLab event body")
	})

	t.Run("unsigned unserved event learns nothing", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.bearer("Pipeline Hook", "", fixture(t, "push"))
		wantReject(t, "unauthenticated unserved event", err, reasonSignature)
		if rg.gl.calls() != 0 {
			t.Fatalf("an unauthenticated delivery must cost no API call, got %d", rg.gl.calls())
		}
	})
}
