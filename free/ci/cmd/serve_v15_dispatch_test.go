package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestDispatchPipelineSecurityWiring(t *testing.T) {
	clone, prepare, execute := dispatchCloneRef, dispatchPreparePlan, dispatchRun
	t.Cleanup(func() { dispatchCloneRef, dispatchPreparePlan, dispatchRun = clone, prepare, execute })
	for _, tc := range []struct {
		name         string
		role         roles.Role
		kind         model.JobKind
		secret, want bool
		webhook      bool
	}{
		{"member release denied", roles.Member, "release", false, false, false},
		{"member secret denied", roles.Member, "test", true, false, false},
		{"release authorized", roles.Release, "release", false, true, false},
		{"ordinary submitted run", roles.Member, "test", false, true, false},
		{"owner API secret", roles.Admin, "test", true, true, false},
		{"owner API ordinary", roles.Admin, "test", false, true, false},
		{"owner webhook secret", roles.Admin, "test", true, true, true},
		{"owner webhook release", roles.Admin, "release", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, home := context.Background(), t.TempDir()
			st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			svc := roles.New(st, &audit.Log{St: st})
			err = svc.Grant(ctx, svc.Operator("local"), roles.Request{Principal: "bob", Role: tc.role, Projects: []string{"o/r"}})
			_ = st.Close()
			if err != nil {
				t.Fatal(err)
			}
			sha := strings.Repeat("a", 40)
			var row store.IntakeRow
			backend := clientapi.StoreBackend{Enqueue: func(_ context.Context, d coordinator.Delivery) (coordinator.Outcome, error) {
				row = store.IntakeRow{Source: d.Source, Project: d.Project, Revision: d.Revision, Facts: d.Facts}
				return coordinator.Outcome{}, nil
			}}
			if _, err := backend.Submit(ctx, clientapi.SubmitRequest{Project: "o/r", Revision: sha}, "user:bob"); err != nil {
				t.Fatal(err)
			}
			trigger, user := "api", "bob"
			if tc.webhook {
				trigger, user = "push", ""
				raw, err := json.Marshal(model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Repo: "o/r", Revision: sha, Kind: "push", Auth: "per-repo"})
				if err != nil {
					t.Fatal(err)
				}
				row.Source, row.Facts = "github", raw
			}
			dispatchCloneRef = func(url, revision, workdir string) (string, error) {
				if url != "https://github.com/o/r.git" || revision != sha {
					t.Fatalf("wrong checkout: %s %s", url, revision)
				}
				return t.TempDir(), nil
			}
			dispatchPreparePlan = func(_ context.Context, o RunOptions) (*plan.Plan, string, func(plan.PlannedJob) bool, error) {
				if !o.TeamRun || o.Trigger != trigger || o.User != user || o.Team != "o" {
					t.Fatalf("missing submitted TEAM context: %+v", o)
				}
				j := model.Job{Kind: tc.kind}
				if tc.secret {
					j.Secrets = []string{"release-key"}
				}
				return &plan.Plan{Jobs: []plan.PlannedJob{{Job: j}}}, "digest", nil, nil
			}
			called := false
			dispatchRun = func(ctx context.Context, c *cirun.Coordinator) (cirun.Result, error) {
				called = true
				if c.Opts.ReleaseAuthorized != (!tc.webhook && (tc.kind == "release" || tc.secret)) {
					t.Fatalf("wrong release authorization: %+v", c.Opts)
				}
				classifier, ok := c.Hooks.Trust.(forgeDispatchTrust)
				if !ok {
					t.Fatalf("dispatch did not install forge trust: %T", c.Hooks.Trust)
				}
				classifier.hook.Getenv = func(string) string { return "" }
				refetched := false
				classifier.hook.Refetch = func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
					refetched = true
					if f.Source != "github" || f.Repo != "o/r" || f.Revision != sha {
						t.Fatalf("wrong forge query: %+v", f)
					}
					f.ActorPermission, f.SHAReachable = apiDispatchFact("admin"), apiDispatchFact(true)
					return f, nil
				}
				got, err := classifier.Classify(ctx, cirun.TrustRequest{Root: c.Opts.Root, Revision: sha})
				wantClass := model.TrustClass("internal")
				if tc.role == roles.Admin {
					wantClass = "owner"
				}
				if err != nil || got != wantClass || !refetched {
					t.Fatalf("class=%s err=%v refetched=%v; want %s", got, err, refetched, wantClass)
				}
				return cirun.Result{}, nil
			}
			_, err = dispatchPipeline(home, &v15ServeFlags{})(ctx, row)
			if (err == nil) != tc.want || called != tc.want {
				t.Fatalf("ran=%v, err=%v, want ran=%v", called, err, tc.want)
			}
			// D-1534: a member's job that reads secrets is refused with E677 before any lease or value;
			// a member's release job keeps the role refusal.
			if wantE677 := tc.role == roles.Member && tc.secret && tc.kind != "release"; (secrets.CodeOf(err) == "E677") != wantE677 {
				t.Fatalf("err=%v; want E677=%v", err, wantE677)
			}
		})
	}
}
