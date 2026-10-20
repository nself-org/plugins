package run

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestCoordinatorPolicyUsesHome(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "explicit"}[explicit], func(t *testing.T) {
			ctx := context.Background()
			root, st, db := runScopeRepo(t)
			t.Setenv("NSELF_CI_HOME", t.TempDir())
			if err := st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: "trust.secrets", Value: []byte(`["project"]`), Actor: "test"}, nil); err != nil {
				t.Fatal(err)
			}
			home := filepath.Dir(db)
			opts := Options{Root: root}
			if explicit {
				opts.Home = home
			} else {
				t.Setenv("NSELF_CI_HOME", home)
			}
			c := Coordinator{Opts: opts, Plan: buildPlan(t, map[string]model.Job{"test": sh("true")})}
			expected, err := (PolicyHook{OpenStore: func() (*store.Store, error) { return store.Open(db, store.Options{}) }}).Resolve(ctx, PolicyRequest{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.policy(ctx, Hooks{Policy: PolicyHook{}})
			if err != nil {
				t.Fatal(err)
			}
			if got.Digest != expected.Digest {
				t.Fatal("policy resolved from a different home")
			}
		})
	}
}
