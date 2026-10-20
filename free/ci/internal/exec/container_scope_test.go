//go:build !windows

package exec

// Purpose: the baseline container honours the effective scope the agent now passes (D-1331): internet and lan
// use the runtime's default network (raw access needs no enforcement), none keeps --network=none, and a scope the
// baseline cannot enforce (restricted, privileged) is refused before the runtime is called.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func baselineScopeSpec(id string, scope model.NetworkScope) JobSpec {
	return JobSpec{AttemptID: id, CoordinatorID: "coord", Home: "", Network: scope,
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"true"}}
}

func TestBaselineContainerScopes(t *testing.T) {
	for _, tc := range []struct {
		scope   model.NetworkScope
		none    bool
		refused bool
	}{{"", true, false}, {"none", true, false}, {"internet", false, false}, {"lan", false, false}, {"restricted", false, true}, {"privileged", false, true}} {
		t.Run(string(tc.scope), func(t *testing.T) {
			calls := fakeRuntime(t, "", "0")
			spec := baselineScopeSpec("scope-"+string(tc.scope), tc.scope)
			spec.Home = t.TempDir()
			res := (&Executor{Env: AllowlistEnv{}}).Run(context.Background(), spec)
			var e *Error
			if tc.refused {
				if !errors.As(res.Err, &e) || e.Code != "E613" {
					t.Fatalf("scope %q: want E613, got %v", tc.scope, res.Err)
				}
				if got := calls(); len(got) != 1 || got[0] != "" {
					t.Fatalf("scope %q: the runtime was called: %q", tc.scope, got)
				}
				return
			}
			if res.Err != nil {
				t.Fatalf("scope %q: %v", tc.scope, res.Err)
			}
			run := strings.Join(calls(), "\n")
			if !strings.Contains(run, "run ") || strings.Contains(run, "--network=none") != tc.none {
				t.Fatalf("scope %q: runtime calls %q (want --network=none: %v)", tc.scope, run, tc.none)
			}
		})
	}
}
