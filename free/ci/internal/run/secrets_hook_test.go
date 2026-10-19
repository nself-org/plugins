package run

// Purpose: a job that declares `secrets:` gets the value in its own environment through the installed
// Source, the value is masked in every output, and an unavailable cli fails the job before it starts.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

const hookValue = "run-hook-fake-value-4d2a91"

// fakeCli speaks contract:cli.secret-resolve v1 for project/FOO only.
type fakeCli struct {
	noCommand bool
	calls     int
}

func (f *fakeCli) Run(_ context.Context, c secrets.Call) (secrets.Output, error) {
	f.calls++
	last := c.Argv[len(c.Argv)-1]
	if last == "--help" {
		if f.noCommand {
			return secrets.Output{Bytes: []byte("unknown command"), ExitCode: 1}, nil
		}
		return secrets.Output{Bytes: []byte("usage")}, nil
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve", "data": map[string]any{
		"schema": "cli.secret-resolve/v1", "attempt_id": c.Argv[indexOf(c.Argv, "--for-attempt")+1],
		"values": []map[string]any{{"ref": last, "env_name": "FOO", "value": hookValue, "source": "age:ci.age", "expires_at": nil}}}})
	return secrets.Output{Bytes: b}, nil
}

func indexOf(l []string, w string) int {
	for i, s := range l {
		if s == w {
			return i
		}
	}
	return -1
}

func secretJob(script string) model.Job {
	j := sh(script)
	j.Secrets = []string{"project/FOO"}
	return j
}

func TestSecretSourceDeliversEnvAndMasksOutput(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`test "$FOO" = "` + hookValue + `" || exit 9; echo "got=[$FOO]"`)}, gitFake{})
	r.C.Opts.Stream = true
	o := outcome(r.run(t), "a")
	if o.State != StatePassed {
		t.Fatalf("job did not get its secret: %+v\n%s", o, r.Out)
	}
	for name, got := range map[string]string{"tail": o.Tail, "stream": r.Out.String()} {
		if strings.Contains(got, hookValue) {
			t.Fatalf("%s leaks the value: %q", name, got)
		}
	}
	if cli.calls != 2 {
		t.Fatalf("cli calls %d, want probe + resolve", cli.calls)
	}
}

func TestSecretSourceUnavailableFailsBeforeStart(t *testing.T) {
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: &fakeCli{noCommand: true}})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ran > ran.txt`)}, gitFake{})
	o := outcome(r.run(t), "a")
	if o.State == StatePassed || !strings.Contains(o.Code+o.Detail+o.Tail, "E675") {
		t.Fatalf("expected E675 before start: %+v", o)
	}
}

func TestSecretSourceEnforcesPolicyAllowlist(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ran`)}, gitFake{})
	r.C.Hooks.Policy = fixedPolicy{PolicyResult{Digest: "d", SecretsInfo: SecretsInfo{SecretClasses: []string{"release"}, HasSecretClasses: true}}}
	o := outcome(r.run(t), "a")
	if o.State == StatePassed || cli.calls != 0 {
		t.Fatalf("a project secret ran under a release-only policy: %+v calls=%d", o, cli.calls)
	}
}

type fixedPolicy struct{ res PolicyResult }

func (f fixedPolicy) Resolve(context.Context, PolicyRequest) (PolicyResult, error) { return f.res, nil }

func TestSecretSourceRefusesForeignRedactor(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ran`)}, gitFake{})
	r.C.Hooks.NewRedactor = func() ciexec.Redactor { return redact.New().NewFilter() } // a filter over another registry
	o := outcome(r.run(t), "a")
	if o.State == StatePassed || cli.calls != 0 {
		t.Fatalf("resolved with a redactor that does not read the registry: %+v calls=%d", o, cli.calls)
	}
}

func TestAgentNodeWithoutFactsResolvesNothing(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModeAgentNode, NodeEligible: true, NodeID: "n1", AgeKeyPath: "/node/age.key", Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ran`)}, gitFake{})
	o := outcome(r.run(t), "a")
	if o.State == StatePassed || cli.calls != 0 {
		t.Fatalf("an agent node without its own facts used the operator's: %+v calls=%d", o, cli.calls)
	}
}

func TestResolverCallCarriesTheLiveCoordinator(t *testing.T) {
	if coordinatorOf(AttemptInfo{}) != "secrets" || coordinatorOf(AttemptInfo{SecretsInfo: SecretsInfo{CoordinatorID: "live-1"}}) != "live-1" {
		t.Fatal("the nested resolver call must carry the live coordinator id")
	}
}

// D-1070: the Source's evidence label (`age:<file>`, `legacy-env`) reaches the check in the run evidence,
// beside the ref and never the value.
func TestSecretSourcesReachRunEvidence(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ok`), "b": sh(`echo plain`)}, gitFake{})
	var ev model.Evidence
	r.C.Hooks.AfterRuns = []AfterRunHook{afterFn(func(ri RunInfo) error { ev = ri.Evidence; return nil })}
	r.run(t)
	var got, plain map[string]string
	for _, c := range ev.Checks {
		switch c.JobID {
		case "a":
			got = c.SecretSources
		case "b":
			plain = c.SecretSources
		}
	}
	if got["project/FOO"] != "age:ci.age" || len(got) != 1 {
		t.Fatalf("check a secret_sources = %v, want project/FOO -> age:ci.age", got)
	}
	if len(plain) != 0 {
		t.Fatalf("a job without secrets carries sources: %v", plain)
	}
	body, err := json.Marshal(ev)
	if err != nil || strings.Contains(string(body), hookValue) {
		t.Fatalf("evidence leaks the value or does not marshal: %v", err)
	}
}
