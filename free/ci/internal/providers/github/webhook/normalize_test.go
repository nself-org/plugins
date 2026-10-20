package webhook

// Purpose: tests of event normalization and the stale-delivery check (acceptance lines 3 and 4).
// Inputs:  the recorded payloads and the recorded API responses of the fake GitHub.
// Outputs: pass or fail; goldens under testdata/golden (rewrite with -update, then review the diff).
// Constraints: each test's doc comment cites the acceptance line it proves.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// declineGolden is how a dropped delivery is recorded.
type declineGolden struct {
	Declined string `json:"declined"`
	E706     string `json:"e706"`
}

// TestNormalizeGoldens proves acceptance line 3: push, tag push, create(tag), pull_request
// opened/synchronize/reopened (other actions dropped with 200 accepted:false), repository_dispatch
// with a 40-hex client_payload.revision (revision_source caller; without it E706 unsupported),
// workflow_job queued (kind runner_demand with labels, job id, run id), workflow_dispatch and every
// other unserved event dropped, branch-delete pushes dropped as today.
func TestNormalizeGoldens(t *testing.T) {
	pr := fixture(t, "pr_fork_opened.json")
	jobDone := replace(fixture(t, "workflow_job_queued.json"), `"action":"queued"`, `"action":"completed"`)
	createBranch := replace(fixture(t, "create_tag.json"), `"ref_type":"tag"`, `"ref_type":"branch"`)
	cases := []struct {
		name, event string
		body        []byte
	}{
		{"push", "push", fixture(t, "push.json")},
		{"push_tag", "push", fixture(t, "push_tag.json")},
		{"push_delete", "push", fixture(t, "push_delete.json")},
		{"create_tag", "create", fixture(t, "create_tag.json")},
		{"create_branch", "create", createBranch},
		{"pr_opened", "pull_request", pr},
		{"pr_synchronize", "pull_request", replace(pr, `"action":"opened"`, `"action":"synchronize"`)},
		{"pr_reopened", "pull_request", replace(pr, `"action":"opened"`, `"action":"reopened"`)},
		{"pr_closed", "pull_request", replace(pr, `"action":"opened"`, `"action":"closed"`)},
		{"repository_dispatch", "repository_dispatch", fixture(t, "repository_dispatch.json")},
		{"repository_dispatch_norev", "repository_dispatch", fixture(t, "repository_dispatch_norev.json")},
		{"workflow_job_queued", "workflow_job", fixture(t, "workflow_job_queued.json")},
		{"workflow_job_completed", "workflow_job", jobDone},
		{"workflow_dispatch", "workflow_dispatch", fixture(t, "workflow_dispatch.json")},
		{"ping", "ping", []byte(`{"zen":"synthetic","repository":{"id":1001,"full_name":"o/r"}}`)},
		{"malformed", "push", []byte(`{"ref":["not","a","string"],"repository":{"id":1001,"full_name":"o/r"}}`)},
	}
	r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	r.env["SYNTH_REPO_SECRET"] = repoSecret
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := r.deliver(c.event, repoSecret, c.body)
			var d *Decline
			if errors.As(err, &d) {
				golden(t, c.name, declineGolden{Declined: d.Reason, E706: d.Err.Reason})
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if verr := f.Validate(); verr != nil {
				t.Fatalf("%s: facts do not validate: %v", c.name, verr)
			}
			assertPayloadSources(t, f)
			golden(t, c.name, f)
		})
	}
	t.Run("kinds and revision sources", func(t *testing.T) {
		for name, want := range map[string]struct {
			kind model.TriggerKind
			rev  string
			src  model.FactSource
		}{
			"push": {"push", shaA, "payload"}, "push_tag": {"tag", shaA, "payload"}, "create_tag": {"tag", shaA, "payload"},
			"repository_dispatch": {"api", shaD, "caller"}, "workflow_job_queued": {"runner_demand", shaC, "payload"},
		} {
			for _, c := range cases {
				if c.name != name {
					continue
				}
				f, err := r.deliver(c.event, repoSecret, c.body)
				if err != nil || f.Kind != want.kind || f.Revision != want.rev || f.RevisionSource != want.src {
					t.Fatalf("%s = %+v, %v", name, f, err)
				}
			}
		}
	})
	t.Run("a runner demand carries labels, job id and run id", func(t *testing.T) {
		f, err := r.deliver("workflow_job", repoSecret, fixture(t, "workflow_job_queued.json"))
		if err != nil || f.RunnerLabels.Value == nil || len(*f.RunnerLabels.Value) != 4 || *f.ProviderJobID.Value != "4001" || *f.ProviderRunID.Value != "3001" {
			t.Fatalf("demand = %+v, %v", f, err)
		}
	})
}

// assertPayloadSources checks that no fact claims an authority it does not have: the payload is the
// only source at normalization, except HeadCurrent which one API call fills.
func assertPayloadSources(t *testing.T, f model.TriggerFacts) {
	t.Helper()
	for name, src := range map[string]string{
		"actor": f.Actor.Source, "bot": f.Bot.Source, "visibility": f.Visibility.Source, "head_repo": f.HeadRepo.Source,
		"fork": f.Fork.Source, "assoc": f.AuthorAssociation.Source, "labels": f.RunnerLabels.Source, "job": f.ProviderJobID.Source,
	} {
		if src != "" && src != "payload" {
			t.Fatalf("fact %s has source %q, want payload", name, src)
		}
	}
	for name, v := range map[string]bool{
		"actor_permission": f.ActorPermission.Value != nil, "first_time": f.FirstTime.Value != nil, "protected": f.ProtectedRef.Value != nil,
		"sha_reachable": f.SHAReachable.Value != nil, "changed_files": f.ChangedFiles.Value != nil, "run_event": f.RunEvent.Value != nil,
	} {
		if v {
			t.Fatalf("fact %s must stay unknown until Refetch", name)
		}
	}
	if f.HeadCurrent.Value != nil && f.HeadCurrent.Source != "api" {
		t.Fatalf("head_current source = %q", f.HeadCurrent.Source)
	}
}

// TestStaleDelivery proves acceptance line 4: a replayed push whose ref now points elsewhere is
// rejected with trigger.stale (E706 stale) after one API call; a moved pull request head and a moved
// tag are stale the same way; with the API failing, timing out or unconfigured the trigger continues
// with HeadCurrent unknown.
func TestStaleDelivery(t *testing.T) {
	newR := func(t *testing.T) *rig {
		r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		return r
	}
	wantStale := func(t *testing.T, err error) {
		t.Helper()
		var d *Decline
		if !errors.As(err, &d) || d.Reason != "trigger.stale" || d.Err.Code != "E706" || d.Err.Reason != "stale" {
			t.Fatalf("want E706 trigger.stale decline, got %v", err)
		}
	}
	t.Run("push whose ref moved", func(t *testing.T) {
		r := newR(t)
		r.gh.set("/repos/o/r/git/ref/heads/main", 200, `{"object":{"type":"commit","sha":"`+shaD+`"}}`)
		_, err := r.deliver("push", repoSecret, fixture(t, "push.json"))
		wantStale(t, err)
		if r.gh.calls() != 1 {
			t.Fatalf("the check is one API call, got %d", r.gh.calls())
		}
	})
	t.Run("pull request head moved", func(t *testing.T) {
		r := newR(t)
		r.gh.set("/repos/o/r/pulls/7", 200, `{"head":{"sha":"`+shaD+`"}}`)
		_, err := r.deliver("pull_request", repoSecret, fixture(t, "pr_fork_opened.json"))
		wantStale(t, err)
	})
	t.Run("tag moved", func(t *testing.T) {
		r := newR(t)
		r.gh.set("/repos/o/r/git/ref/tags/v1.0.0", 200, `{"object":{"type":"tag","sha":"`+shaD+`"}}`)
		_, err := r.deliver("push", repoSecret, fixture(t, "push_tag.json"))
		wantStale(t, err)
	})
	t.Run("current head sets HeadCurrent from the api", func(t *testing.T) {
		r := newR(t)
		f, err := r.deliver("push", repoSecret, fixture(t, "push.json"))
		if err != nil || f.HeadCurrent.Value == nil || !*f.HeadCurrent.Value || f.HeadCurrent.Source != "api" {
			t.Fatalf("current head = %+v, %v", f.HeadCurrent, err)
		}
	})
	t.Run("api 500 continues with HeadCurrent unknown", func(t *testing.T) {
		r := newR(t)
		r.gh.set("/repos/o/r/git/ref/heads/main", 500, `{"message":"boom"}`)
		f, err := r.deliver("push", repoSecret, fixture(t, "push.json"))
		if err != nil || f.HeadCurrent.Value != nil || f.Revision != shaA {
			t.Fatalf("api failure = %+v, %v", f, err)
		}
	})
	t.Run("a 404 can mean no access and is unknown, not stale", func(t *testing.T) {
		r := newR(t)
		r.gh.drop("/repos/o/r/git/ref/heads/main")
		f, err := r.deliver("push", repoSecret, fixture(t, "push.json"))
		if err != nil || f.HeadCurrent.Value != nil {
			t.Fatalf("404 = %+v, %v", f.HeadCurrent, err)
		}
	})
	t.Run("no api client continues", func(t *testing.T) {
		r := newR(t)
		r.src.NewAPI = func(context.Context) (*rest.Client, error) { return nil, errors.New("no credential") }
		if f, err := r.deliver("push", repoSecret, fixture(t, "push.json")); err != nil || f.HeadCurrent.Value != nil {
			t.Fatalf("no client = %+v, %v", f.HeadCurrent, err)
		}
	})
	t.Run("the check stops at its budget", func(t *testing.T) {
		old := staleBudget
		staleBudget = 60 * time.Millisecond
		t.Cleanup(func() { staleBudget = old })
		r := newR(t)
		r.gh.slow("/repos/o/r/git/ref/heads/main", 2*time.Second)
		start := time.Now()
		f, err := r.deliver("push", repoSecret, fixture(t, "push.json"))
		if err != nil || f.HeadCurrent.Value != nil || time.Since(start) > time.Second {
			t.Fatalf("slow api = %+v, %v after %v", f.HeadCurrent, err, time.Since(start))
		}
	})
}
