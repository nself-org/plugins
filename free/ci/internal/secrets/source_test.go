package secrets

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// operatorFacts is the operator's own personal node running its own release-capable job.
func operatorFacts() Facts {
	return Facts{
		Job:    trust.JobFacts{Trust: "internal", PersonalOwnRun: true, Mode: trust.Personal},
		Runner: trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: true, Dedicated: true, Isolation: "process", UIDSeparation: true},
	}
}

func newSrc(f *fakeNself, mutate func(*Config)) (*Source, *recorder) {
	rec := &recorder{}
	cfg := Config{Mode: ModePersonalLocal, Attempt: "att-1", Resolver: &Resolver{Runner: f, Project: "/work/proj"}, Registry: rec,
		RedactorInstalled: func() bool { return true }, Facts: operatorFacts()}
	if mutate != nil {
		mutate(&cfg)
	}
	return New(cfg), rec
}

func TestSourcePersonalLocalResolvesAndRegisters(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted, "release/BAR": "second-fake-value-55"})
	s, rec := newSrc(f, nil)
	got, err := s.Resolve(context.Background(), []string{"project/FOO", "release/BAR"})
	if err != nil || got["FOO"] != planted || got["BAR"] != "second-fake-value-55" || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if rec.count() != 2 {
		t.Fatalf("registered %d values before returning, want 2", rec.count())
	}
	calls := f.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls %v", calls)
	}
	argv := strings.Join(calls[1].Argv, " ")
	if argv != "nself config secrets resolve --project /work/proj --for-attempt att-1 --json project/FOO release/BAR" || calls[1].Timeout != CallTimeout {
		t.Fatalf("argv %q timeout %v", argv, calls[1].Timeout)
	}
	if strings.Contains(argv, planted) || s.Sources()["project/FOO"] != "age:ci.age" || s.Held() != 2 {
		t.Fatalf("held %d sources %v", s.Held(), s.Sources())
	}
	snap := append([][]byte(nil), s.bufs...)
	s.Zero()
	s.Zero()
	for _, b := range snap {
		if strings.Trim(string(b), "\x00") != "" {
			t.Fatal("buffer not zeroed")
		}
	}
	if s.Held() != 0 {
		t.Fatal("held after Zero")
	}
}

func TestSourceNoRefsNoWork(t *testing.T) {
	f := newFake(nil)
	s, _ := newSrc(f, nil)
	if got, err := s.Resolve(context.Background(), nil); got != nil || err != nil || len(f.Calls()) != 0 {
		t.Fatal("work for no refs")
	}
}

func TestPassThroughRedactorBlocksResolution(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, rec := newSrc(f, func(c *Config) { c.RedactorInstalled = func() bool { return false } })
	got, err := s.Resolve(context.Background(), []string{"project/FOO"})
	if got != nil || CodeOf(err) != "E676" || len(f.Calls()) != 0 || rec.count() != 0 {
		t.Fatalf("got=%v err=%v calls=%d", got, err, len(f.Calls()))
	}
	s2, _ := newSrc(f, func(c *Config) { c.RedactorInstalled = nil })
	if _, err = s2.Resolve(context.Background(), []string{"project/FOO"}); CodeOf(err) != "E676" || len(f.Calls()) != 0 {
		t.Fatalf("nil check must fail closed: %v", err)
	}
}

// valueResolver hands out a value the test keeps a reference to.
type valueResolver struct{ v []byte }

func (valueResolver) Probe(context.Context) error { return nil }
func (r valueResolver) Resolve(_ context.Context, _ string, refs []Ref) ([]Value, error) {
	return []Value{{Ref: refs[0], Bytes: r.v, Source: "age:ci.age"}}, nil
}

func TestRegisterErrorFailsBeforeStartAndInjectsNothing(t *testing.T) {
	short := []byte("abc")
	s, _ := newSrc(newFake(nil), func(c *Config) { c.Resolver = valueResolver{short} })
	got, err := s.Resolve(context.Background(), []string{"project/FOO"})
	if got != nil || CodeOf(err) != "E676" || s.Held() != 0 || strings.Contains(err.Error(), "abc") {
		t.Fatalf("got=%v err=%v held=%d", got, err, s.Held())
	}
	if strings.Trim(string(short), "\x00") != "" {
		t.Fatal("rejected value not zeroed")
	}
}

func TestNotInPolicyMakesNoCall(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, _ := newSrc(f, func(c *Config) { c.Facts.Allowed, c.Facts.HasPolicy = []string{"release"}, true })
	_, err := s.Resolve(context.Background(), []string{"project/FOO"})
	var e *Error
	if CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, string(trust.SecretsNotInPolicy)) || len(f.Calls()) != 0 {
		t.Fatalf("%v calls=%d", err, len(f.Calls()))
	}
	s, _ = newSrc(f, func(c *Config) { c.Facts.Allowed, c.Facts.HasPolicy = []string{"project"}, true })
	if _, err = s.Resolve(context.Background(), []string{"project/FOO"}); err != nil {
		t.Fatalf("allowed class refused: %v", err)
	}
}

func asErr(err error, out **Error) bool { e, ok := err.(*Error); *out = e; return ok }
func has(list []string, w string) bool {
	for _, s := range list {
		if s == w {
			return true
		}
	}
	return false
}

func TestTeamCoordinatorPassesRefsOnly(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, rec := newSrc(f, func(c *Config) { c.Mode = ModeTeamCoordinator })
	got, err := s.Resolve(context.Background(), []string{"project/FOO"})
	var e *Error
	if got != nil || CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, ReasonRefsOnly) {
		t.Fatalf("%v %v", got, err)
	}
	refs, err := s.Refs([]string{"project/FOO", "release/BAR"})
	if err != nil || len(refs) != 2 || refs[0].Class != "project" || refs[1].Ref != "release/BAR" {
		t.Fatalf("%v %v", refs, err)
	}
	if len(f.Calls()) != 0 || s.Held() != 0 || rec.count() != 0 {
		t.Fatal("a coordinator resolved or held something")
	}
	// the executing node's Source resolves.
	node, _ := newSrc(f, func(c *Config) {
		c.Mode, c.NodeEligible, c.NodeID, c.AgeKeyPath = ModeAgentNode, true, "n1", "/node/age.key"
	})
	if got, err = node.Resolve(context.Background(), []string{"project/FOO"}); err != nil || got["FOO"] != planted {
		t.Fatalf("%v %v", got, err)
	}
}

func TestNodeNotEligibleSource(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, _ := newSrc(f, func(c *Config) {
		c.Mode, c.NodeEligible, c.NodeID, c.AgeKeyPath = ModeAgentNode, false, "node-7", "/node/age.key"
	})
	got, err := s.Resolve(context.Background(), []string{"project/FOO"})
	var e *Error
	if got != nil || CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, ReasonNodeNotEligible) || !strings.Contains(err.Error(), "node-7") {
		t.Fatalf("%v %v", got, err)
	}
	if len(f.Calls()) != 0 || s.Held() != 0 {
		t.Fatal("an ineligible node made a resolver call or held a value")
	}
}

func TestAgentNodeUsesItsOwnIdentity(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	res := &Resolver{Runner: f, Project: "/p", AgeKeyPath: "/var/lib/node/age.key"}
	s, _ := newSrc(f, func(c *Config) {
		c.Mode, c.NodeEligible, c.NodeID, c.AgeKeyPath, c.Resolver = ModeAgentNode, true, "n1", "/var/lib/node/age.key", res
	})
	if _, err := s.Resolve(context.Background(), []string{"project/FOO"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls() {
		if len(c.Env) != 2 || c.Env[0] != "NSELF_V15=1" || c.Env[1] != "SECRETS_AGE_KEY_PATH=/var/lib/node/age.key" || strings.Contains(strings.Join(c.Env, ""), "operator") {
			t.Fatalf("env %v", c.Env)
		}
	}
	none, _ := newSrc(f, func(c *Config) { c.Mode, c.NodeEligible, c.AgeKeyPath = ModeAgentNode, true, "" })
	if _, err := none.Resolve(context.Background(), []string{"project/FOO"}); CodeOf(err) != "E676" {
		t.Fatalf("agent node without identity: %v", err)
	}
}

func TestResolverUnavailableAndProbedOnce(t *testing.T) {
	f := newFake(nil)
	f.hasCmd = false
	shared := &ProbeState{}
	for i := 0; i < 3; i++ {
		s, _ := newSrc(f, func(c *Config) { c.Resolver = &Resolver{Runner: f, Project: "/p", Shared: shared} })
		if _, err := s.Resolve(context.Background(), []string{"project/FOO"}); CodeOf(err) != "E675" {
			t.Fatalf("want E675, got %v", err)
		}
	}
	if len(f.Calls()) != 1 || f.resolveCalls() != 0 {
		t.Fatalf("probe ran %d times", len(f.Calls()))
	}
}

func TestRefCollisionIsE674WithoutCalls(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, _ := newSrc(f, nil)
	if _, err := s.Resolve(context.Background(), []string{"project/FOO", "release/FOO"}); CodeOf(err) != "E674" || len(f.Calls()) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestReleaseRefRefusals(t *testing.T) {
	f := newFake(map[string]string{"release/ANDROID_KEYSTORE_PASSWORD": planted})
	// a container runner that is not dedicated-trusted
	s, _ := newSrc(f, func(c *Config) {
		c.Facts.Runner = trust.RunnerFacts{Ownership: "operator", Isolation: "container", UIDSeparation: true}
	})
	_, err := s.Resolve(context.Background(), []string{"release/ANDROID_KEYSTORE_PASSWORD"})
	var e *Error
	if CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, string(trust.TrustReleaseNeedsDedicated)) {
		t.Fatalf("%v", err)
	}
	// a runner that does not declare the release class
	s, _ = newSrc(f, func(c *Config) {
		c.Facts.Declared, c.Facts.HasDeclared = []model.SecretClass{"project"}, true
	})
	if _, err = s.Resolve(context.Background(), []string{"release/ANDROID_KEYSTORE_PASSWORD"}); CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, string(trust.SecretsClassNotDeclared)) {
		t.Fatalf("%v", err)
	}
	if len(f.Calls()) != 0 {
		t.Fatal("refused attempt called the resolver")
	}
	// the operator's own personal node resolves it
	s, _ = newSrc(f, nil)
	if got, err := s.Resolve(context.Background(), []string{"release/ANDROID_KEYSTORE_PASSWORD"}); err != nil || got["ANDROID_KEYSTORE_PASSWORD"] != planted {
		t.Fatalf("%v %v", got, err)
	}
}
