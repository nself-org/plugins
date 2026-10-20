package webhook

// Purpose: regression tests added after the independent code review of P7-HOST-05: the fork
// definition, push HeadCurrent on refetch, absent claims staying unknown, annotated tags, rename
// paths, escaping of odd ref names and the --insecure edge cases.
// Inputs:  the recorded payloads with small edits, and routes set on the fake GitHub.
// Outputs: pass or fail.
// Constraints: each case names the review finding it pins.

import (
	"context"
	"strings"
	"testing"
)

// TestForkMeansAnotherRepository pins the review MUST-FIX: a same-repository pull request in a
// repository that is itself a fork (head.id == base.id, head.fork true) is not a fork pull request, in
// the payload fact, the refetched pull request and the refetched workflow run.
func TestForkMeansAnotherRepository(t *testing.T) {
	r := refetchRig(t)
	same := `{"number":7,"author_association":"MEMBER","head":{"ref":"fix","sha":"` + shaC + `","repo":{"id":1001,"full_name":"o/r","fork":true}},"base":{"ref":"main","sha":"` + shaB + `","repo":{"id":1001,"full_name":"o/r"}}}`
	r.gh.set("/repos/o/r/pulls/7", 200, same)
	r.gh.set("/repos/o/r/actions/runs/3001", 200, `{"event":"pull_request","actor":{"login":"newbie"},"repository":{"id":1001,"full_name":"o/r"},"head_repository":{"id":1001,"full_name":"o/r","fork":true}}`)
	body := replace(fixture(t, "pr_fork_opened.json"), `"repo":{"id":2002,"full_name":"newbie/r","private":false,"fork":true}`, `"repo":{"id":1001,"full_name":"o/r","private":false,"fork":true}`)
	f, err := r.deliver("pull_request", repoSecret, body)
	if err != nil || f.Fork.Value == nil || *f.Fork.Value {
		t.Fatalf("payload fork = %+v, %v", f.Fork, err)
	}
	got, err := r.src.Refetch(context.Background(), f)
	if err != nil || got.Fork.Value == nil || *got.Fork.Value || got.Fork.Source != "api" {
		t.Fatalf("refetched fork = %+v, %v", got.Fork, err)
	}
	demand, err := r.src.Refetch(context.Background(), deliveredFacts(t, r, "workflow_job", "workflow_job_queued.json"))
	if err != nil || demand.Fork.Value == nil || *demand.Fork.Value {
		t.Fatalf("run fork = %+v, %v", demand.Fork, err)
	}
}

// TestRefetchReviewFixes pins the review SHOULD-FIX items on Refetch.
func TestRefetchReviewFixes(t *testing.T) {
	ctx := context.Background()
	t.Run("a pushed branch that moved is no longer current", func(t *testing.T) {
		r := refetchRig(t)
		in := deliveredFacts(t, r, "push", "push.json")
		got, err := r.src.Refetch(ctx, in)
		if err != nil || got.HeadCurrent.Value == nil || !*got.HeadCurrent.Value {
			t.Fatalf("current push = %+v, %v", got.HeadCurrent, err)
		}
		r.gh.set("/repos/o/r/git/ref/heads/main", 200, `{"object":{"type":"commit","sha":"`+shaD+`"}}`)
		got, err = r.src.Refetch(ctx, in)
		if err != nil || got.HeadCurrent.Value == nil || *got.HeadCurrent.Value || got.HeadCurrent.Source != "api" {
			t.Fatalf("moved push = %+v, %v", got.HeadCurrent, err)
		}
	})
	t.Run("a rename lists both paths", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.set("/repos/o/r/compare/"+shaB+"..."+shaC, 200, `{"files":[{"filename":"docs/x.md","previous_filename":".github/workflows/x.yml"}]}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil || got.ChangedFiles.Value == nil || strings.Join(*got.ChangedFiles.Value, ",") != "docs/x.md,.github/workflows/x.yml" {
			t.Fatalf("rename = %+v, %v", got.ChangedFiles, err)
		}
	})
	t.Run("a missing base does not prove the head unreachable", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.drop("/repos/o/r/compare/" + shaB + "..." + shaC)
		r.gh.set("/repos/o/r/compare/main..."+shaC, 200, `{"status":"behind","files":[]}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil || got.SHAReachable.Value == nil || !*got.SHAReachable.Value || got.ChangedFiles.Value != nil {
			t.Fatalf("garbage-collected base = %+v %+v, %v", got.SHAReachable, got.ChangedFiles, err)
		}
	})
	t.Run("an empty 200 invents nothing", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.set("/repos/o/r", 200, `{}`)
		r.gh.set("/repos/o/r/pulls/7", 200, `{}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got.Visibility.Source != "payload" || got.HeadCurrent.Value != nil || got.HeadRepo.Source != "payload" || got.Fork.Source != "payload" {
			t.Fatalf("empty responses must leave facts as the payload had them: vis=%+v cur=%+v head=%+v fork=%+v", got.Visibility, got.HeadCurrent, got.HeadRepo, got.Fork)
		}
	})
}

// TestNormalizeReviewFixes pins the review SHOULD-FIX items on normalization and Verify.
func TestNormalizeReviewFixes(t *testing.T) {
	newR := func(t *testing.T) *rig {
		r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		return r
	}
	t.Run("a fork branch called main is not a push to main", func(t *testing.T) {
		r := newR(t)
		body := replace(fixture(t, "pr_fork_opened.json"), `"head":{"ref":"fix"`, `"head":{"ref":"main"`)
		f, err := r.deliver("pull_request", repoSecret, body)
		if err != nil || f.Ref != "refs/pull/7/head" || f.BaseRef != "refs/heads/main" {
			t.Fatalf("PR refs = %q / %q, %v", f.Ref, f.BaseRef, err)
		}
		d, err := r.deliver("workflow_job", repoSecret, fixture(t, "workflow_job_queued.json"))
		if err != nil || d.Ref != "" {
			t.Fatalf("a runner demand must not carry the sender's branch name: %q, %v", d.Ref, err)
		}
	})
	t.Run("an annotated tag push without head_commit is dereferenced", func(t *testing.T) {
		r := newR(t)
		body := replace(fixture(t, "push_tag.json"), `"head_commit":{"id":"`+shaA+`","message":"synthetic release"}`, `"head_commit":null`)
		f, err := r.deliver("push", repoSecret, body)
		if err != nil || f.Kind != "tag" || f.Revision != shaA {
			t.Fatalf("tag without head_commit = %+v, %v", f, err)
		}
		if r.gh.hits["/repos/o/r/git/tags/"+strings.Repeat("e", 40)] == 0 {
			t.Fatal("the tag object must be dereferenced through the API when head_commit is missing")
		}
		r.gh.drop("/repos/o/r/git/tags/" + strings.Repeat("e", 40))
		if _, err = r.deliver("push", repoSecret, body); err == nil || !strings.Contains(err.Error(), "tag.unresolved") {
			t.Fatalf("an undereferenceable tag must decline, got %v", err)
		}
	})
	t.Run("create(tag) with a failing api declines", func(t *testing.T) {
		r := newR(t)
		r.gh.failAll()
		_, err := r.deliver("create", repoSecret, fixture(t, "create_tag.json"))
		if err == nil || !strings.Contains(err.Error(), "tag.unresolved") {
			t.Fatalf("create(tag) = %v", err)
		}
	})
	t.Run("odd branch names reach the api escaped one segment at a time", func(t *testing.T) {
		r := newR(t)
		branch := "feature/a b%2e?#x"
		r.gh.set("/repos/o/r/git/ref/heads/"+branch, 200, `{"object":{"type":"commit","sha":"`+shaA+`"}}`)
		body := replace(fixture(t, "push.json"), `"ref":"refs/heads/main"`, `"ref":"refs/heads/`+branch+`"`)
		f, err := r.deliver("push", repoSecret, body)
		if err != nil || f.HeadCurrent.Value == nil || !*f.HeadCurrent.Value {
			t.Fatalf("odd branch = %+v, %v", f.HeadCurrent, err)
		}
		want := "/repos/o/r/git/ref/heads/feature/a%20b%252e%3F%23x"
		if got := r.gh.paths[len(r.gh.paths)-1]; got != want {
			t.Fatalf("api path = %q, want %q", got, want)
		}
	})
	t.Run("a dot segment branch is never sent", func(t *testing.T) {
		r := newR(t)
		body := replace(fixture(t, "push.json"), `"ref":"refs/heads/main"`, `"ref":"refs/heads/a/../b"`)
		f, err := r.deliver("push", repoSecret, body)
		if err != nil || f.HeadCurrent.Value != nil || r.gh.calls() != 0 {
			t.Fatalf("dot segment = %+v, %v, calls %d", f.HeadCurrent, err, r.gh.calls())
		}
	})
	t.Run("absent visibility stays unknown", func(t *testing.T) {
		r := newR(t)
		body := replace(fixture(t, "push.json"), `"private":false,"visibility":"public",`, ``)
		f, err := r.deliver("push", repoSecret, body)
		if err != nil || f.Visibility.Value != nil {
			t.Fatalf("visibility = %+v, %v", f.Visibility, err)
		}
	})
	t.Run("a runner demand with no labels carries an empty list, not null", func(t *testing.T) {
		r := newR(t)
		body := replace(fixture(t, "workflow_job_queued.json"), `"labels":["self-hosted","nself","linux","x64"]`, `"labels":[]`)
		f, err := r.deliver("workflow_job", repoSecret, body)
		if err != nil || f.RunnerLabels.Value == nil || *f.RunnerLabels.Value == nil {
			t.Fatalf("labels = %+v, %v", f.RunnerLabels, err)
		}
	})
	t.Run("a signed ping that names no repository is acknowledged", func(t *testing.T) {
		r := newR(t)
		r.env[SharedSecretEnv] = sharedSecret
		_, err := r.deliver("ping", sharedSecret, []byte(`{"zen":"synthetic","hook_id":1}`))
		if err == nil || !strings.Contains(err.Error(), "declined") {
			t.Fatalf("org ping = %v", err)
		}
	})
}

// TestInsecureNeverReplacesAnOwnSecret pins the review test gap: --insecure only excuses a repository
// that has no secret of its own; with one, an unsigned or wrongly signed delivery is still E706.
func TestInsecureNeverReplacesAnOwnSecret(t *testing.T) {
	r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	r.env["SYNTH_REPO_SECRET"] = repoSecret
	r.src.Insecure = func() bool { return true }
	push := fixture(t, "push.json")
	_, err := r.deliver("push", "", push)
	wantE706(t, err, "unsigned delivery for a repository with its own secret under --insecure")
	_, err = r.deliver("push", "synthetic-wrong-secret", push)
	wantE706(t, err, "wrongly signed delivery under --insecure")
}

// TestRepoNameLengthBound pins the QA finding: ValidProject has no length cap, so an authenticated
// delivery naming a 10 KB repository is refused here.
func TestRepoNameLengthBound(t *testing.T) {
	r := newRig(t, nil)
	r.env[SharedSecretEnv] = sharedSecret
	long := strings.Repeat("a", 10<<10)
	body := replace(fixture(t, "push.json"), `"full_name":"o/r"`, `"full_name":"o/`+long+`"`)
	_, err := r.deliver("push", sharedSecret, body)
	wantE706(t, err, "10 KB repository name")
}

// TestOpusReviewFixes pins the adversarial review: reachability reads the compare status (a 200 for an
// unmerged commit is not reachable), a replayed workflow_job queued event is stale once the job ran,
// a dispatch says nothing about the head repository, a configured id beats a stale pin, a shared
// delivery never pins, and a per-repo project keeps one spelling.
func TestOpusReviewFixes(t *testing.T) {
	ctx := context.Background()
	t.Run("an unmerged commit is not reachable, a merged one is", func(t *testing.T) {
		r := refetchRig(t)
		for status, want := range map[string]bool{"ahead": false, "diverged": false, "behind": true, "identical": true} {
			r.gh.set("/repos/o/r/compare/main..."+shaA, 200, `{"status":"`+status+`","files":[]}`)
			got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "push", "push.json"))
			if err != nil || got.SHAReachable.Value == nil || *got.SHAReachable.Value != want {
				t.Fatalf("status %s: reachable = %+v, %v (want %v)", status, got.SHAReachable, err, want)
			}
		}
	})
	t.Run("a dispatch revision from the fork network reads as unreachable with unknown head repo", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.set("/repos/o/r/compare/main..."+shaD, 200, `{"status":"diverged","files":[]}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "repository_dispatch", "repository_dispatch.json"))
		if err != nil || got.SHAReachable.Value == nil || *got.SHAReachable.Value || got.HeadRepo.Value != nil || got.Fork.Value != nil {
			t.Fatalf("dispatch = reach %+v head %+v fork %+v, %v", got.SHAReachable, got.HeadRepo, got.Fork, err)
		}
	})
	t.Run("a replayed queued job is stale", func(t *testing.T) {
		for _, status := range []string{"in_progress", "completed"} {
			r := refetchRig(t)
			r.gh.set("/repos/o/r/actions/jobs/4001", 200, `{"status":"`+status+`"}`)
			_, err := r.deliver("workflow_job", repoSecret, fixture(t, "workflow_job_queued.json"))
			if err == nil || !strings.Contains(err.Error(), "trigger.stale") {
				t.Fatalf("job %s = %v", status, err)
			}
		}
		r := refetchRig(t)
		r.gh.set("/repos/o/r/actions/jobs/4001", 500, `{}`)
		if f, err := r.deliver("workflow_job", repoSecret, fixture(t, "workflow_job_queued.json")); err != nil || f.Kind != "runner_demand" {
			t.Fatalf("api failure must continue: %+v, %v", f, err)
		}
	})
	t.Run("a configured id replaces a stale pin and shared deliveries never pin", func(t *testing.T) {
		r := newRig(t, nil)
		r.env[SharedSecretEnv] = sharedSecret
		poison := replace(fixture(t, "push.json"), `"repository":{"id":1001`, `"repository":{"id":1`)
		if _, err := r.deliver("push", sharedSecret, poison); err != nil {
			t.Fatal(err)
		}
		cfg := map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}}
		r.src.Secrets = func(context.Context) (map[string]RepoSecret, error) { return cfg, nil }
		r.env["SYNTH_REPO_SECRET"] = repoSecret
		// a pin written by the shared delivery (id 1) would refuse the real id 1001 here
		if f, err := r.deliver("push", repoSecret, fixture(t, "push.json")); err != nil || f.Auth != "per-repo" {
			t.Fatalf("real delivery after a shared-secret poison attempt = %+v, %v", f, err)
		}
		// the per-repo delivery pinned 1001; a configured id now decides alone, whatever the pin says
		cfg = map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET", ID: 2002}}
		moved := replace(fixture(t, "push.json"), `"repository":{"id":1001`, `"repository":{"id":2002`)
		if f, err := r.deliver("push", repoSecret, moved); err != nil || f.RepoID != "2002" {
			t.Fatalf("a configured id must replace the pin: %+v, %v", f, err)
		}
	})
	t.Run("a per-repo project has one spelling", func(t *testing.T) {
		r := refetchRig(t)
		f, err := r.deliver("push", repoSecret, replace(fixture(t, "push.json"), `"full_name":"o/r"`, `"full_name":"O/R"`))
		if err != nil || f.Repo != "o/r" {
			t.Fatalf("repo = %q, %v", f.Repo, err)
		}
	})
}

// TestCrossFamilyReviewFixes pins the valid findings of the cross-family review: a null-SHA dispatch
// revision is refused, SHA comparison ignores hex case, a tag that moved is no longer current on
// refetch, and a protected-branch lookup keeps the slash of a branch name (GitHub accepts the literal
// slash on the branches, git/ref and compare endpoints, checked against the public API).
func TestCrossFamilyReviewFixes(t *testing.T) {
	ctx := context.Background()
	r := refetchRig(t)
	zero := replace(fixture(t, "repository_dispatch.json"), shaD, strings.Repeat("0", 40))
	if _, err := r.deliver("repository_dispatch", repoSecret, zero); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("null revision = %v", err)
	}
	r.gh.set("/repos/o/r/git/ref/heads/main", 200, `{"object":{"type":"commit","sha":"`+strings.ToUpper(shaA)+`"}}`)
	if f, err := r.deliver("push", repoSecret, fixture(t, "push.json")); err != nil || f.HeadCurrent.Value == nil || !*f.HeadCurrent.Value {
		t.Fatalf("upper-case sha from the api = %+v, %v", f.HeadCurrent, err)
	}
	tag := deliveredFacts(t, r, "push", "push_tag.json")
	got, err := r.src.Refetch(ctx, tag)
	if err != nil || got.HeadCurrent.Value == nil || !*got.HeadCurrent.Value {
		t.Fatalf("current tag = %+v, %v", got.HeadCurrent, err)
	}
	r.gh.set("/repos/o/r/git/tags/"+strings.Repeat("e", 40), 200, `{"object":{"type":"commit","sha":"`+shaD+`"}}`)
	got, err = r.src.Refetch(ctx, tag)
	if err != nil || got.HeadCurrent.Value == nil || *got.HeadCurrent.Value {
		t.Fatalf("moved tag = %+v, %v", got.HeadCurrent, err)
	}
	r.gh.set("/repos/o/r/branches/release/v1", 200, `{"protected":true}`)
	body := replace(fixture(t, "pr_fork_opened.json"), `"base":{"ref":"main"`, `"base":{"ref":"release/v1"`)
	f, err := r.deliver("pull_request", repoSecret, body)
	if err != nil {
		t.Fatal(err)
	}
	got, err = r.src.Refetch(ctx, f)
	if err != nil || got.ProtectedRef.Value == nil || !*got.ProtectedRef.Value {
		t.Fatalf("slashed base branch = %+v, %v", got.ProtectedRef, err)
	}
}
