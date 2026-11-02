package main

import (
	"context"
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

func TestDispatchPipelineSecurityWiring(t *testing.T) {
	clone, prepare, execute := dispatchCloneRef, dispatchPreparePlan, dispatchRun
	t.Cleanup(func() { dispatchCloneRef, dispatchPreparePlan, dispatchRun = clone, prepare, execute })
	for _, tc := range []struct {
		name         string
		role         roles.Role
		kind         model.JobKind
		secret, want bool
	}{
		{"member release denied", roles.Member, "release", false, false},
		{"member secret denied", roles.Member, "test", true, false},
		{"release authorized", roles.Release, "release", false, true},
		{"ordinary submitted run", roles.Member, "test", false, true},
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
			row := store.IntakeRow{Source: "manual", Project: "o/r", Revision: sha,
				Facts: []byte(`{"trigger":"api","submitter":"user:bob"}`)}
			dispatchCloneRef = func(url, revision, workdir string) (string, error) {
				if url != "https://github.com/o/r.git" || revision != sha {
					t.Fatalf("wrong checkout: %s %s", url, revision)
				}
				return t.TempDir(), nil
			}
			dispatchPreparePlan = func(_ context.Context, o RunOptions) (*plan.Plan, string, func(plan.PlannedJob) bool, error) {
				if !o.TeamRun || o.Trigger != "api" || o.User != "bob" || o.Team != "o" {
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
				if c.Opts.ReleaseAuthorized != (tc.kind == "release" || tc.secret) {
					t.Fatalf("wrong release authorization: %+v", c.Opts)
				}
				classifier, ok := c.Hooks.Trust.(forgeDispatchTrust)
				if !ok {
					t.Fatalf("dispatch did not install forge trust: %T", c.Hooks.Trust)
				}
				classifier.hook.Getenv = func(string) string { return "" }
				got, err := classifier.Classify(ctx, cirun.TrustRequest{Root: c.Opts.Root, Revision: sha})
				if err != nil || got != "untrusted" {
					t.Fatalf("legacy API source must not inherit local owner: %s %v", got, err)
				}
				return cirun.Result{}, nil
			}
			_, err = dispatchPipeline(home, &v15ServeFlags{})(ctx, row)
			if (err == nil) != tc.want || called != tc.want {
				t.Fatalf("ran=%v, err=%v, want ran=%v", called, err, tc.want)
			}
		})
	}
}
