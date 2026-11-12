package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func apiDispatchFact[T any](v T) model.Fact[T] { return model.Fact[T]{Value: &v, Source: "api"} }

func TestDispatchForgeTrust(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, kind, want                    string
		fork, reachable, unavailable, stale bool
	}{
		{"verified push", "push", "owner", false, true, false, false},
		{"fork pull request", "pull_request", "untrusted", true, false, false, false},
		{"unreviewed pull request", "pull_request", "collaborator", false, false, false, false},
		{"admin reachable pull request", "pull_request", "collaborator", false, true, false, false},
		{"forge unavailable", "push", "untrusted", false, true, true, false},
		{"cached authority only", "push", "untrusted", false, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Repo: "o/r", Revision: sha,
				Kind: model.TriggerKind(tc.kind), Auth: "per-repo", ActorPermission: apiDispatchFact("admin"),
				SHAReachable: apiDispatchFact(true), ProtectedRef: apiDispatchFact(true), Fork: apiDispatchFact(false)}
			raw, _ := json.Marshal(original)
			facts, err := dispatchTrustFacts(store.IntakeRow{Source: "github", Project: "o/r", Revision: sha, Facts: raw}, pipelineFacts{})
			if err != nil {
				t.Fatal(err)
			}
			called := false
			h := forgeDispatchTrust{facts: facts, hook: cirun.SourceTrust{Getenv: func(string) string { return "" },
				Refetch: func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
					called = true
					if f.Source != "github" || f.Repo != "o/r" || f.Revision != sha || f.ActorPermission.Value != nil || f.SHAReachable.Value != nil || f.ProtectedRef.Value != nil {
						t.Fatalf("refetch was given stale authority or wrong source: %+v", f)
					}
					if tc.unavailable {
						return f, errors.New("forge unavailable")
					}
					if !tc.stale {
						f.ActorPermission, f.SHAReachable, f.Fork = apiDispatchFact("admin"), apiDispatchFact(tc.reachable), apiDispatchFact(tc.fork)
					}
					return f, nil
				}}}
			got, err := h.Classify(context.Background(), cirun.TrustRequest{Root: t.TempDir(), Revision: sha})
			if err != nil || string(got) != tc.want || !called {
				t.Fatalf("class = %s, %v, refetched=%v; want %s", got, err, called, tc.want)
			}
			called = false
			if got, err := h.Classify(context.Background(), cirun.TrustRequest{Revision: strings.Repeat("b", 40)}); got != "untrusted" || err == nil || called {
				t.Fatalf("different checkout SHA must fail before refetch: %s, %v", got, err)
			}
		})
	}
}

func TestDispatchFactsBindIntake(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"legacy", `{"trigger":"api","submitter":"user:bob"}`, true},
		{"bad JSON", `{`, false},
		{"wrong source", `{"schema":"ci.trigger-facts/v1","source":"gitlab"}`, false},
		{"wrong repo", `{"schema":"ci.trigger-facts/v1","repo":"evil/fork"}`, false},
		{"wrong revision", `{"schema":"ci.trigger-facts/v1","revision":"bbbb"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var legacy pipelineFacts
			_ = json.Unmarshal([]byte(tc.raw), &legacy)
			got, err := dispatchTrustFacts(store.IntakeRow{Source: "github", Project: "o/r", Revision: sha, Facts: []byte(tc.raw)}, legacy)
			if (err == nil) != tc.want {
				t.Fatalf("facts accepted=%v, want %v: %v", err == nil, tc.want, err)
			}
			if err == nil && (got.Source != "github" || got.Repo != "o/r" || got.Revision != sha || got.Kind != "api") {
				t.Fatalf("legacy facts not pinned to intake: %+v", got)
			}
		})
	}
}

func TestDispatchReleaseAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, label, project string
		kind                 model.JobKind
		secret, want         bool
	}{
		{"member ordinary", "user:member", "o/r", "test", false, true},
		{"member release", "user:member", "o/r", "release", false, false},
		{"member secret", "user:member", "o/r", "test", true, false},
		{"release job", "user:releaser", "o/r", "release", false, true},
		{"release secret", "user:releaser", "o/r", "test", true, true},
		{"release wrong project", "user:releaser", "other/repo", "release", false, false},
		{"webhook release", "", "o/r", "release", false, true},
		{"webhook secret", "", "o/r", "test", true, true},
		{"webhook ordinary", "", "o/r", "test", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := cirun.OpenStore(filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			svc := roles.New(st, &audit.Log{St: st})
			ctx := context.Background()
			for user, role := range map[string]roles.Role{"member": roles.Member, "releaser": roles.Release} {
				if err := svc.Grant(ctx, svc.Operator("local"), roles.Request{Principal: user, Role: role, Projects: []string{"o/r"}}); err != nil {
					t.Fatal(err)
				}
			}
			job := model.Job{Kind: tc.kind}
			if tc.secret {
				job.Secrets = []string{"release-key"}
			}
			// Put an ordinary job first: the gate must scan the complete selected plan.
			p := &plan.Plan{Jobs: []plan.PlannedJob{{Job: model.Job{Kind: "test"}}, {Job: job}}}
			allowed, err := authorizeDispatchRelease(ctx, st, tc.label, tc.project, p)
			if (err == nil) != tc.want || allowed != (tc.want && tc.label != "" && (tc.secret || tc.kind == "release")) {
				t.Fatalf("authorized=%v, err=%v, want success=%v", allowed, err, tc.want)
			}
			rows, err := st.AuditEntries(ctx, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if allowed {
				found := false
				for _, row := range rows {
					found = found || row.Action == "authorize" && row.Target == roles.ActReleaseStart
				}
				if !found {
					t.Fatal("release authorization was not audited")
				}
			}
		})
	}
}

// wire03Class classifies a pull request through the dispatch trust hook with the forge answering perm.
func wire03Class(t *testing.T, perm string, fork bool) model.TrustClass {
	t.Helper()
	sha := strings.Repeat("c", 40)
	h := forgeDispatchTrust{facts: model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Repo: "o/r", Revision: sha, Kind: "pull_request", Auth: "per-repo"},
		hook: cirun.SourceTrust{Getenv: func(string) string { return "" },
			Refetch: func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
				f.ActorPermission, f.SHAReachable, f.Fork = apiDispatchFact(perm), apiDispatchFact(false), apiDispatchFact(fork)
				f.FirstTime = apiDispatchFact(false)
				return f, nil
			}}}
	got, err := h.Classify(context.Background(), cirun.TrustRequest{Root: t.TempDir(), Revision: sha})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWire03SamePRAdminClass(t *testing.T) {
	if got := wire03Class(t, "admin", false); got != "collaborator" {
		t.Fatalf("same-repo admin PR class = %s, want collaborator (never owner, never the fork class)", got)
	}
}

func TestWire03PRNeverOwnerOrAdminClassForOthers(t *testing.T) {
	for name, tc := range map[string]struct {
		perm string
		fork bool
	}{"admin fork": {"admin", true}, "write same-repo": {"write", false}, "read same-repo": {"read", false}, "none": {"", false}} {
		if got := wire03Class(t, tc.perm, tc.fork); got != "untrusted" {
			t.Errorf("%s: class = %s, want untrusted", name, got)
		}
	}
}
