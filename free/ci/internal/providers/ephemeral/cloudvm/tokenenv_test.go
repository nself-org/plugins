package cloudvm_test

// Purpose: TestTokenEnv*: the provider token reaches the infra child through NSELF_CI_BURST_TOKEN only,
// read from the token file at call time; it is never on argv, never in the coordinator's own
// environment and never in a job environment.
// Inputs:  the rig (fake nself records every child's argv and environment). Outputs: pass/fail.
// Constraints: synthetic values only; the flag spelling is built in pieces so the source holds no literal of it.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
)

var bareTokenFlag = "--to" + "ken"

func envNames(env []string) []string {
	var out []string
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		out = append(out, k)
	}
	return out
}

func TestTokenEnvChildEnvironmentIsTheTokenOnly(t *testing.T) {
	t.Setenv("SOME_CLOUD_TOKEN", "outer-sentinel-value")
	r := newRig(t)
	ctx := context.Background()
	res, err := r.prov.Provision(ctx, intentOf("01J0000000000000000000ENV01"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.prov.List(ctx, map[string]string{"role": "ci-ephemeral"}); err != nil {
		t.Fatal(err)
	}
	if err = r.prov.Teardown(ctx, res.ID); err != nil {
		t.Fatal(err)
	}
	calls := r.br.recorded()
	if len(calls) < 4 {
		t.Fatalf("expected the probe, list, provision, list and destroy children, got %d", len(calls))
	}
	for _, c := range calls {
		for _, name := range envNames(c.Env) {
			if name != cloudvm.TokenEnv && name != "HOME" && name != "NSELF_CI_EXEC_ATTEMPT" { // the funnel's own marker is the only addition
				t.Errorf("%v: the child inherited %s", c.Argv, name)
			}
		}
		for _, a := range c.Argv {
			if a == bareTokenFlag || strings.Contains(a, fakeToken) {
				t.Errorf("%v: the token is on the command line", c.Argv)
			}
		}
		if !envHas(c.Env, "HOME="+pluginHome) { // the nself plugin router falls back to /tmp when HOME is unset
			t.Errorf("%v: HOME is not the assigned plugin home: %v", c.Argv, envNames(c.Env))
		}
		serverVerb := len(c.Argv) > 3 && c.Argv[3] != "--help"
		if serverVerb != envHas(c.Env, cloudvm.TokenEnv+"="+fakeToken) {
			t.Errorf("%v: token env present=%v", c.Argv, !serverVerb)
		}
		if serverVerb && !(len(c.Argv) > 2 && c.Argv[len(c.Argv)-2] == "--token-env" && c.Argv[len(c.Argv)-1] == cloudvm.TokenEnv) {
			t.Errorf("%v: --token-env NSELF_CI_BURST_TOKEN must close the argv", c.Argv)
		}
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, cloudvm.TokenEnv+"=") || strings.Contains(e, fakeToken) {
			t.Fatalf("the coordinator's own environment holds the token: %s", strings.SplitN(e, "=", 2)[0])
		}
	}
}

func TestTokenEnvIsAbsentFromJobsOnTheCoordinator(t *testing.T) {
	r := newRig(t)
	if _, err := r.prov.Provision(context.Background(), intentOf("01J0000000000000000000ENV02")); err != nil {
		t.Fatal(err)
	}
	envBin, err := exec.LookPath("env")
	if err != nil {
		t.Skip("no env binary")
	}
	var mu sync.Mutex
	var out strings.Builder
	spec := ciexec.JobSpec{AttemptID: "tokenenv-job", CoordinatorID: "tokenenv", Command: []string{envBin}, Home: filepath.Join(t.TempDir(), "ci"),
		Output: func(b []byte) { mu.Lock(); out.Write(b); mu.Unlock() }}
	if res := (&ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: ciexec.AllowlistEnv{}}).Run(context.Background(), spec); res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("job: %+v", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(out.String(), "NSELF_CI_BURST_TOKEN") || strings.Contains(out.String(), fakeToken) {
		t.Fatalf("a job on the coordinator sees the token variable:\n%s", out.String())
	}
}

func TestTokenEnvIsReadAtCallTime(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.prov.List(ctx, map[string]string{"role": "ci-ephemeral"}); err != nil {
		t.Fatal(err)
	}
	const rotated = "synthetic-token-0002-rotated"
	if err := os.WriteFile(r.tokenFile, []byte(rotated), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.prov.List(ctx, map[string]string{"role": "ci-ephemeral"}); cloudvm.ReasonOf(err) != cloudvm.ReasonTokenRefused {
		t.Fatalf("the rotated token is not accepted yet, the provider must name the refusal: %v", err)
	}
	r.br.set(func(k *knobs) { k.token = rotated })
	if _, err := r.prov.List(ctx, map[string]string{"role": "ci-ephemeral"}); err != nil {
		t.Fatalf("the new token must be read from the file without a restart: %v", err)
	}
	last := r.br.callsOf("list")
	if !envHas(last[len(last)-1].Env, "NSELF_CI_BURST_TOKEN="+rotated) {
		t.Fatal("the child did not get the rotated token")
	}
}
