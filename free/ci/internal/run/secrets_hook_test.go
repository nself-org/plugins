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

// The live coordinator id is "<boot>:<pid>:<start>"; the executor refuses ids with ':' (E612), so the resolver
// call carries the same safe form jobs carry. Before this, every resolver call under a real coordinator failed.
func TestResolverCoordinatorIDIsAnExecutorSafeID(t *testing.T) {
	live := "8a1f3c52-9d0e-4e4b-b7d1-0c2f6a5e1b77:48213:1760892291"
	got := coordinatorOf(AttemptInfo{SecretsInfo: SecretsInfo{CoordinatorID: live}})
	if got != safeID(live) || strings.Contains(got, ":") {
		t.Fatalf("coordinator id %q", got)
	}
	var ex ciexec.Executor
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: "sec-x-1", CoordinatorID: got, Command: []string{"true"}, Home: t.TempDir()})
	if res.Err != nil {
		t.Fatalf("the executor refused the coordinator id %q: %v", got, res.Err)
	}
}

// D-1325: the local facts follow the class the coordinator gave the source. A class outside what the local
// coordinator runs (owner, internal) gets no project secret, so a later change that lets collaborator work
// run locally cannot inherit internal-level secrets.
func TestLocalFactsFollowTheSourceClass(t *testing.T) {
	refs, err := secrets.ParseAll([]string{"project/FOO"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		source   model.TrustClass
		wantTrst model.TrustClass
		allowed  bool
	}{
		{"owner", "owner", true},
		{"internal", "internal", true},
		{"", "internal", true}, // an attempt built without a class keeps the local default
		{"collaborator", "collaborator", false},
		{"untrusted", "untrusted", false},
	} {
		f := localFacts(AttemptInfo{Source: c.source})
		if f.Job.Trust != c.wantTrst {
			t.Errorf("source %q: job trust %q, want %q", c.source, f.Job.Trust, c.wantTrst)
		}
		if got := len(secrets.Check(refs, f)) == 0; got != c.allowed {
			t.Errorf("source %q: project secret allowed=%v, want %v", c.source, got, c.allowed)
		}
	}
}

// The coordinator hands each attempt the class its classifier returned (the source of the run).
func TestAttemptCarriesTheClassifiedSource(t *testing.T) {
	var got []model.TrustClass
	r := newRig(t, map[string]model.Job{"a": secretJob(`echo ran`)}, gitFake{})
	r.C.Hooks.Trust = fakeTrust("internal")
	r.C.Hooks.NewSecrets = func(a AttemptInfo, _ func() ciexec.Redactor) ciexec.SecretSource {
		got = append(got, a.Source)
		return nil
	}
	outcome(r.run(t), "a")
	if len(got) == 0 || got[0] != "internal" {
		t.Fatalf("attempt source classes %v, want [internal]", got)
	}
}
