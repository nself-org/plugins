package exec

// Purpose: the two exec seams the ad-hoc job uses (P7-CI-52): a read-only
// workspace mount passes the container backstop (and only in its exact form),
// and a per-job EnvPolicy replaces the executor's.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestCheckMountReadOnlyWorkspace(t *testing.T) {
	const ws = "/w/ws"
	for spec, ok := range map[string]bool{
		"type=bind,src=/w/ws,dst=/repo":                             true,
		"type=bind,src=/w/ws,dst=/repo,readonly=true":               true,
		"type=bind,src=/w/ws,dst=/repo,readonly=false":              false,
		"type=bind,src=/w/ws,dst=/repo,readonly":                    false,
		"type=bind,src=/w/ws,dst=/repo,readonly=true,ro=true":       false,
		"type=bind,src=/w/ws,dst=/repo,readonly=true,readonly=true": false,
		"type=bind,src=/w/ws,dst=/repo,bind-propagation=shared":     false,
		"type=bind,src=/w/ws,dst=/etc,readonly=true":                false,
		"type=bind,src=/etc,dst=/repo,readonly=true":                false,
	} {
		if err := checkMount(spec, ws); (err == nil) != ok {
			t.Errorf("checkMount(%q) = %v, want ok=%v", spec, err, ok)
		}
	}
}

type fixedEnv []string

func (f fixedEnv) Environment(context.Context, model.Job) ([]string, error) { return f, nil }

func TestJobSpecEnvPolicyOverridesExecutor(t *testing.T) {
	t.Setenv("CI52_HOST_ONLY", "from-host")
	var out strings.Builder
	run := func(spec JobSpec) string {
		out.Reset()
		spec.AttemptID, spec.CoordinatorID, spec.Home = "env-"+strings.ReplaceAll(t.Name(), "/", "-"), "coord", t.TempDir()
		spec.Output = func(b []byte) { out.Write(b) }
		spec.Command = []string{"/usr/bin/env"}
		if res := (&Executor{Isolation: BaselineIsolator{}, Env: inheritEnvironment{}}).Run(context.Background(), spec); res.Err != nil || res.ExitCode != 0 {
			t.Fatalf("run: %v exit %d", res.Err, res.ExitCode)
		}
		return out.String()
	}
	if got := run(JobSpec{}); !strings.Contains(got, "CI52_HOST_ONLY=from-host") {
		t.Fatalf("the executor policy did not apply:\n%s", got)
	}
	got := run(JobSpec{Env: fixedEnv{"ONLY=this"}, Job: model.Job{Env: map[string]string{"DECLARED": "yes"}}})
	if strings.Contains(got, "CI52_HOST_ONLY") || !strings.Contains(got, "ONLY=this") || !strings.Contains(got, "DECLARED=yes") {
		t.Fatalf("the per-job policy must replace the executor's and keep declared env:\n%s", got)
	}
}
