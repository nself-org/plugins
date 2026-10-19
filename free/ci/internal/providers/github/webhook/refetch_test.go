package webhook

// Purpose: tests of the TrustFacts refetch (acceptance line 5).
// Inputs:  the recorded fork pull request and runner demand deliveries, the recorded API responses.
// Outputs: pass or fail.
// Constraints: each test's doc comment cites the acceptance line it proves.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func deliveredFacts(t *testing.T, r *rig, event, fixtureName string) model.TriggerFacts {
	t.Helper()
	f, err := r.deliver(event, repoSecret, fixture(t, fixtureName))
	if err != nil {
		t.Fatalf("%s: %v", fixtureName, err)
	}
	return f
}

func refetchRig(t *testing.T) *rig {
	r := newRig(t, map[string]RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}})
	r.env["SYNTH_REPO_SECRET"] = repoSecret
	return r
}

// TestRefetch proves acceptance line 5: Refetch on a fork PR fixture yields Fork true (api), HeadRepo
// the fork, ActorPermission read, FirstTime per author_association FIRST_TIME_CONTRIBUTOR; a payload
// claiming protected_ref true for an unprotected branch ends with ProtectedRef false (api); Refetch
// on a runner_demand fixture whose run event is pull_request_target yields RunEvent
// pull_request_target and the fork HeadRepo. A failing API never invents a fact.
func TestRefetch(t *testing.T) {
	ctx := context.Background()
	t.Run("fork pull request", func(t *testing.T) {
		r := refetchRig(t)
		in := deliveredFacts(t, r, "pull_request", "pr_fork_opened.json")
		claimed := true
		in.ProtectedRef = model.Fact[bool]{Value: &claimed, Source: "payload", ObservedAt: fixedNow, Confidence: "known"}
		got, err := r.src.Refetch(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err = got.Validate(); err != nil {
			t.Fatalf("refetched facts do not validate: %v", err)
		}
		checks := map[string]bool{
			"fork":       got.Fork.Value != nil && *got.Fork.Value && got.Fork.Source == "api",
			"head repo":  got.HeadRepo.Value != nil && *got.HeadRepo.Value == "newbie/r" && got.HeadRepo.Source == "api",
			"permission": got.ActorPermission.Value != nil && *got.ActorPermission.Value == "read" && got.ActorPermission.Source == "api",
			"first time": got.FirstTime.Value != nil && *got.FirstTime.Value && got.FirstTime.Source == "api",
			"assoc":      got.AuthorAssociation.Value != nil && *got.AuthorAssociation.Value == "FIRST_TIME_CONTRIBUTOR" && got.AuthorAssociation.Source == "api",
			"protected":  got.ProtectedRef.Value != nil && !*got.ProtectedRef.Value && got.ProtectedRef.Source == "api",
			"reachable":  got.SHAReachable.Value != nil && *got.SHAReachable.Value && got.SHAReachable.Source == "api",
			"current":    got.HeadCurrent.Value != nil && *got.HeadCurrent.Value && got.HeadCurrent.Source == "api",
			"bot":        got.Bot.Value != nil && !*got.Bot.Value && got.Bot.Source == "api",
			"visibility": got.Visibility.Value != nil && *got.Visibility.Value == "public" && got.Visibility.Source == "api",
			"files":      got.ChangedFiles.Value != nil && strings.Join(*got.ChangedFiles.Value, ",") == "a.go,docs/b.md" && got.ChangedFiles.Source == "api",
		}
		for name, ok := range checks {
			if !ok {
				t.Errorf("fact %s wrong in %+v", name, got)
			}
		}
		if in.Fork.Source != "payload" {
			t.Fatal("Refetch must not mutate its input")
		}
	})
	t.Run("push", func(t *testing.T) {
		r := refetchRig(t)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "push", "push.json"))
		if err != nil || got.ActorPermission.Value == nil || *got.ActorPermission.Value != "admin" || got.Fork.Value == nil || *got.Fork.Value || *got.HeadRepo.Value != "o/r" ||
			got.ChangedFiles.Value == nil || len(*got.ChangedFiles.Value) != 1 || got.ProtectedRef.Value == nil || got.ProtectedRef.Source != "api" {
			t.Fatalf("push refetch = %+v, %v", got, err)
		}
	})
	t.Run("runner demand reads the workflow run", func(t *testing.T) {
		r := refetchRig(t)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "workflow_job", "workflow_job_queued.json"))
		if err != nil || got.RunEvent.Value == nil || *got.RunEvent.Value != "pull_request_target" || got.RunEvent.Source != "api" ||
			got.HeadRepo.Value == nil || *got.HeadRepo.Value != "newbie/r" || got.Fork.Value == nil || !*got.Fork.Value ||
			got.Actor.Value == nil || got.Actor.Source != "api" {
			t.Fatalf("demand refetch = %+v, %v", got, err)
		}
		if r.gh.calls() != 1 {
			t.Fatalf("a runner demand reads only the run, got %d calls", r.gh.calls())
		}
	})
	t.Run("a failing api never invents a fact", func(t *testing.T) {
		r := refetchRig(t)
		in := deliveredFacts(t, r, "pull_request", "pr_fork_opened.json")
		claimed := true
		in.ProtectedRef = model.Fact[bool]{Value: &claimed, Source: "payload", ObservedAt: fixedNow, Confidence: "known"}
		r.gh.failAll()
		got, err := r.src.Refetch(ctx, in)
		if err == nil || got.ProtectedRef.Source != "payload" || got.ActorPermission.Value != nil || got.FirstTime.Value != nil {
			t.Fatalf("all failing = %+v, %v", got, err)
		}
	})
	t.Run("one failing call keeps the others", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.set("/repos/o/r/collaborators/newbie/permission", 403, `{"message":"forbidden"}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil || got.ActorPermission.Value != nil || got.Fork.Value == nil || got.Fork.Source != "api" {
			t.Fatalf("partial = %+v, %v", got, err)
		}
	})
	t.Run("a commit the repository does not have is unreachable", func(t *testing.T) {
		r := refetchRig(t)
		r.gh.drop("/repos/o/r/compare/" + shaB + "..." + shaC)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil || got.SHAReachable.Value == nil || *got.SHAReachable.Value || got.ChangedFiles.Value != nil {
			t.Fatalf("unreachable = %+v, %v", got, err)
		}
	})
	t.Run("300 changed files or more is unknown", func(t *testing.T) {
		r := refetchRig(t)
		var files []string
		for i := 0; i < maxChangedFiles; i++ {
			files = append(files, fmt.Sprintf(`{"filename":"f%d.go"}`, i))
		}
		r.gh.set("/repos/o/r/compare/"+shaB+"..."+shaC, 200, `{"files":[`+strings.Join(files, ",")+`]}`)
		got, err := r.src.Refetch(ctx, deliveredFacts(t, r, "pull_request", "pr_fork_opened.json"))
		if err != nil || got.ChangedFiles.Value != nil || got.SHAReachable.Value == nil || !*got.SHAReachable.Value {
			t.Fatalf("300 files = %+v, %v", got.ChangedFiles, err)
		}
	})
	t.Run("another source's facts pass through untouched", func(t *testing.T) {
		r := refetchRig(t)
		f := model.TriggerFacts{Source: "gitlab", Repo: "o/r"}
		got, err := r.src.Refetch(ctx, f)
		if err != nil || got.Source != "gitlab" || r.gh.calls() != 0 {
			t.Fatalf("foreign facts = %+v, %v, calls %d", got, err, r.gh.calls())
		}
	})
}
