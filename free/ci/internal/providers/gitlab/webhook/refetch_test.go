package webhook

// Purpose: trust-fact refetch tests (P7-HOST-21 acceptance line 3): a fork merge request yields Fork
// true, HeadRepo the source project, ActorPermission from the member access level (Developer ->
// write) and ProtectedRef from the branch; push, tag and runner-demand facts; failure handling.
// Inputs:  facts from Verify and the fake API (testdata/api/routes.json, edited per case).
// Outputs: pass or fail.
// Constraints: no live call; routes are keyed by escaped path (+ sorted query).

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func strOf(f model.Fact[string]) string {
	if f.Value == nil {
		return "<unknown>"
	}
	return *f.Value
}

func boolOf(f model.Fact[bool]) string {
	if f.Value == nil {
		return "<unknown>"
	}
	return fmt.Sprint(*f.Value)
}

// TestRefetch proves acceptance line 3: the refetch of a fork merge request fixture yields Fork true,
// HeadRepo the source project, ActorPermission from the member access level (Developer -> write) and
// ProtectedRef from the target branch; the other families (push, tag, demand) and every failure mode.
func TestRefetch(t *testing.T) {
	const (
		forkMR = "/projects/acme%2Fapp/merge_requests/7"
		member = "/projects/acme%2Fapp/members/all/55"
	)
	t.Run("fork merge request", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.set(forkMR, 200, `{"iid":7,"sha":"`+shaD+`","source_project_id":2002,"target_project_id":1001}`)
		in, err := rg.signed("Merge Request Hook", fixture(t, "mr_fork_open"))
		if err != nil {
			t.Fatal(err)
		}
		out, err := rg.src.Refetch(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if boolOf(out.Fork) != "true" || out.Fork.Source != "api" || strOf(out.HeadRepo) != "contrib/app-fork" || out.HeadRepo.Source != "api" {
			t.Fatalf("fork facts: %+v %+v", out.Fork, out.HeadRepo)
		}
		if strOf(out.ActorPermission) != "write" || out.ActorPermission.Source != "api" {
			t.Fatalf("Developer must be write: %+v", out.ActorPermission)
		}
		if boolOf(out.ProtectedRef) != "true" || boolOf(out.HeadCurrent) != "true" || boolOf(out.Bot) != "false" || strOf(out.Visibility) != "private" {
			t.Fatalf("protected/head/bot/visibility: %s %s %s %s", boolOf(out.ProtectedRef), boolOf(out.HeadCurrent), boolOf(out.Bot), strOf(out.Visibility))
		}
		if out.ChangedFiles.Value == nil || strings.Join(*out.ChangedFiles.Value, ",") != "main.go,ci/new.yml,ci/old.yml" {
			t.Fatalf("changed files = %+v", out.ChangedFiles)
		}
		if boolOf(out.SHAReachable) != "false" { // the head is a commit the default branch does not have
			t.Fatalf("reachable = %s", boolOf(out.SHAReachable))
		}
		golden(t, "refetch_fork_mr", out)
	})
	t.Run("access levels", func(t *testing.T) {
		for level, want := range map[int]string{50: "admin", 40: "maintain", 30: "write", 20: "triage", 10: "read", 5: "read", 0: "none"} {
			rg := newRig(t, both())
			rg.gl.set(member, 200, fmt.Sprintf(`{"id":55,"access_level":%d}`, level))
			out, err := rg.src.Refetch(context.Background(), mustFacts(t, rg, "Push Hook", "push"))
			if err != nil || strOf(out.ActorPermission) != want {
				t.Fatalf("level %d = %s (%v), want %s", level, strOf(out.ActorPermission), err, want)
			}
		}
	})
	t.Run("not a member is none only when the project was read", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.drop(member)
		out, _ := rg.src.Refetch(context.Background(), mustFacts(t, rg, "Push Hook", "push"))
		if strOf(out.ActorPermission) != "none" {
			t.Fatalf("a readable project with no membership: %+v", out.ActorPermission)
		}
		rg = newRig(t, both())
		in := mustFacts(t, rg, "Push Hook", "push")
		rg.gl.drop(member)
		rg.gl.drop("/projects/acme%2Fapp")
		out, _ = rg.src.Refetch(context.Background(), in)
		if out.ActorPermission.Value != nil {
			t.Fatalf("a 404 may mean no access: permission must stay unknown, got %+v", out.ActorPermission)
		}
	})
	t.Run("bot accounts", func(t *testing.T) {
		rg := newRig(t, both())
		rg.gl.set("/users?username=dev1", 200, `[{"id":55,"username":"dev1","bot":true}]`)
		if out, _ := rg.src.Refetch(context.Background(), mustFacts(t, rg, "Push Hook", "push")); boolOf(out.Bot) != "true" {
			t.Fatalf("bot flag = %s", boolOf(out.Bot))
		}
		rg = newRig(t, both())
		rg.gl.set("/users?username=dev1", 200, `[{"id":55,"username":"dev1"}]`)
		in := mustFacts(t, rg, "Push Hook", "push")
		in.Actor = fact("project_1001_bot_abcdef", "payload", rg.now)
		rg.gl.set("/users?username=project_1001_bot_abcdef", 200, `[{"id":55,"username":"project_1001_bot_abcdef"}]`)
		if out, _ := rg.src.Refetch(context.Background(), in); boolOf(out.Bot) != "true" {
			t.Fatalf("access token bot name = %s", boolOf(out.Bot))
		}
	})
	t.Run("push and tag", func(t *testing.T) {
		rg := newRig(t, both())
		out, err := rg.src.Refetch(context.Background(), mustFacts(t, rg, "Push Hook", "push"))
		if err != nil {
			t.Fatal(err)
		}
		if strOf(out.HeadRepo) != "acme/app" || boolOf(out.Fork) != "false" || boolOf(out.ProtectedRef) != "true" || boolOf(out.HeadCurrent) != "true" || boolOf(out.SHAReachable) != "true" {
			t.Fatalf("push facts: head %s fork %s protected %s current %s reachable %s", strOf(out.HeadRepo), boolOf(out.Fork), boolOf(out.ProtectedRef), boolOf(out.HeadCurrent), boolOf(out.SHAReachable))
		}
		if out.ChangedFiles.Value == nil || strings.Join(*out.ChangedFiles.Value, ",") != "docs/a.md,new/x.go,old/x.go" {
			t.Fatalf("push changed files (both sides of a rename) = %+v", out.ChangedFiles)
		}
		golden(t, "refetch_push", out)
		tg := mustFacts(t, rg, "Tag Push Hook", "tag_push")
		if out, err = rg.src.Refetch(context.Background(), tg); err != nil || boolOf(out.HeadCurrent) != "true" || strOf(out.HeadRepo) != "acme/app" {
			t.Fatalf("tag facts: %+v %v", out, err)
		}
		rg.gl.set("/projects/acme%2Fapp/repository/tags/v1.0.0", 200, `{"commit":{"id":"`+shaB+`"}}`)
		if out, _ = rg.src.Refetch(context.Background(), tg); boolOf(out.HeadCurrent) != "false" {
			t.Fatalf("a tag that moved: current = %s", boolOf(out.HeadCurrent))
		}
	})
	t.Run("reachability", func(t *testing.T) {
		rg := newRig(t, both())
		in := mustFacts(t, rg, "Push Hook", "push")
		rg.gl.drop("/projects/acme%2Fapp/repository/compare?from=main&to=" + shaA)
		if out, _ := rg.src.Refetch(context.Background(), in); boolOf(out.SHAReachable) != "false" {
			t.Fatalf("404 on a readable project = not in the project: %s", boolOf(out.SHAReachable))
		}
		rg.gl.set("/projects/acme%2Fapp/repository/compare?from=main&to="+shaA, 200, `{"commits":[],"compare_timeout":true}`)
		if out, _ := rg.src.Refetch(context.Background(), in); out.SHAReachable.Value != nil {
			t.Fatalf("a compare that timed out says nothing: %s", boolOf(out.SHAReachable))
		}
	})
	t.Run("changed files: 300 are recorded, 301 are not", func(t *testing.T) {
		rg := newRig(t, both())
		in := mustFacts(t, rg, "Merge Request Hook", "mr_open")
		page := func(n int) string {
			items := make([]string, n)
			for i := range items {
				items[i] = fmt.Sprintf(`{"old_path":"f%d","new_path":"f%d"}`, i, i)
			}
			return "[" + strings.Join(items, ",") + "]"
		}
		key := func(p int) string {
			return "/projects/acme%2Fapp/merge_requests/7/diffs?page=" + strconv.Itoa(p) + "&per_page=100"
		}
		for p := 1; p <= 3; p++ {
			rg.gl.set(key(p), 200, page(100))
		}
		rg.gl.set(key(4), 200, `[]`)
		if out, _ := rg.src.Refetch(context.Background(), in); out.ChangedFiles.Value == nil || len(*out.ChangedFiles.Value) != 300 {
			t.Fatalf("exactly 300 files is complete (Ticket: <= 300): %+v", out.ChangedFiles)
		}
		rg.gl.set(key(4), 200, page(1))
		if out, _ := rg.src.Refetch(context.Background(), in); out.ChangedFiles.Value != nil {
			t.Fatalf("301 files is over the cap: must be unknown, got %d", len(*out.ChangedFiles.Value))
		}
		rg.gl.set(key(4), 200, page(100))
		if out, _ := rg.src.Refetch(context.Background(), in); out.ChangedFiles.Value != nil {
			t.Fatalf("400 files is over the cap: must be unknown")
		}
		rg.gl.set(key(3), 200, page(99))
		if out, _ := rg.src.Refetch(context.Background(), in); out.ChangedFiles.Value == nil || len(*out.ChangedFiles.Value) != 299 {
			t.Fatalf("299 files is complete: %+v", out.ChangedFiles)
		}
		// a push compares base to revision; the compare answer has no paging
		pushIn := mustFacts(t, rg, "Push Hook", "push")
		many := make([]string, 301)
		for i := range many {
			many[i] = fmt.Sprintf(`{"old_path":"g%d","new_path":"g%d"}`, i, i)
		}
		rg.gl.set("/projects/acme%2Fapp/repository/compare?from="+shaB+"&straight=true&to="+shaA, 200, `{"commits":[{}],"diffs":[`+strings.Join(many, ",")+`]}`)
		if out, _ := rg.src.Refetch(context.Background(), pushIn); out.ChangedFiles.Value != nil {
			t.Fatalf("a compare of 301 files is unknown")
		}
	})
	t.Run("runner demand", func(t *testing.T) {
		rg := newRig(t, both())
		in := mustFacts(t, rg, "Job Hook", "job_pending_nself")
		out, err := rg.src.Refetch(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if strOf(out.RunEvent) != "merge_request_event" || out.RunEvent.Source != "api" || boolOf(out.Fork) != "false" || strOf(out.HeadRepo) != "acme/app" || strOf(out.Actor) != "dev1" {
			t.Fatalf("demand facts: event %s fork %s head %s actor %s", strOf(out.RunEvent), boolOf(out.Fork), strOf(out.HeadRepo), strOf(out.Actor))
		}
		rg.gl.set("/projects/acme%2Fapp/merge_requests/7", 200, `{"iid":7,"sha":"`+shaD+`","source_project_id":2002,"target_project_id":1001}`)
		if out, _ = rg.src.Refetch(context.Background(), in); boolOf(out.Fork) != "true" || strOf(out.HeadRepo) != "contrib/app-fork" {
			t.Fatalf("a fork merge request pipeline: fork %s head %s", boolOf(out.Fork), strOf(out.HeadRepo))
		}
		golden(t, "refetch_demand_fork", out)
		rg.gl.set("/projects/acme%2Fapp/pipelines/501", 200, `{"source":"push","ref":"main","user":{"username":"dev1"}}`)
		if out, _ = rg.src.Refetch(context.Background(), in); strOf(out.RunEvent) != "push" || boolOf(out.Fork) != "false" {
			t.Fatalf("a push pipeline runs the project's own code: %s %s", strOf(out.RunEvent), boolOf(out.Fork))
		}
		rg.gl.set("/projects/acme%2Fapp/pipelines/501", 200, `{"source":"external_pull_request_event","ref":"main","user":{"username":"dev1"}}`)
		if out, _ = rg.src.Refetch(context.Background(), in); out.Fork.Value != nil || out.HeadRepo.Value != nil {
			t.Fatalf("an external pull request pipeline says nothing about the source: %s %s", boolOf(out.Fork), strOf(out.HeadRepo))
		}
	})
	t.Run("failures never invent facts", func(t *testing.T) {
		rg := newRig(t, both())
		in := mustFacts(t, rg, "Push Hook", "push")
		rg.gl.failAll()
		out, err := rg.src.Refetch(context.Background(), in)
		if err == nil || out.Fork.Value != nil || out.ProtectedRef.Value != nil || out.ActorPermission.Value != nil || out.ChangedFiles.Value != nil {
			t.Fatalf("every call failed: want the original facts and an error, got %+v err %v", out, err)
		}
		other := in
		other.Source = "github"
		if out, err = rg.src.Refetch(context.Background(), other); err != nil || out.Source != "github" {
			t.Fatalf("another source's facts pass through: %v", err)
		}
		bad := in
		bad.Repo = "../etc"
		if _, err = rg.src.Refetch(context.Background(), bad); err != nil {
			t.Fatalf("an invalid project name passes through untouched: %v", err)
		}
	})
	t.Run("paths are escaped", func(t *testing.T) {
		rg := newRig(t, both())
		if _, err := rg.src.Refetch(context.Background(), mustFacts(t, rg, "Merge Request Hook", "mr_open")); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"/projects/acme%2Fapp", "/projects/acme%2Fapp/repository/branches/main"} {
			if !rg.gl.saw(p) {
				t.Fatalf("expected a call to %s, saw %v", p, rg.gl.paths)
			}
		}
		for _, p := range rg.gl.paths {
			if strings.Contains(p, "acme/app") {
				t.Fatalf("project path not escaped as one segment: %s", p)
			}
		}
	})
}

// mustFacts verifies a recorded delivery and returns its facts.
func mustFacts(t *testing.T, rg *rig, event, name string) model.TriggerFacts {
	t.Helper()
	f, err := rg.signed(event, fixture(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f
}
