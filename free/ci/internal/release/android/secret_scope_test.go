package android

// Purpose: the lane's keystore secrets reach a job only through the personal-node
// compat source (P7-TRUST-02, Epic CACHE D18): a dedicated, secrets-eligible
// agent node that passes every other check still gets no value from the
// operator's environment, and the compat source is never used unaudited.
// Constraints: obviously fake values; no process, no network.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

type absentResolver struct{ calls int }

func (r *absentResolver) Probe(context.Context) error { return nil }
func (r *absentResolver) Resolve(context.Context, string, []secrets.Ref) ([]secrets.Value, error) {
	r.calls++
	return nil, &secrets.Error{Code: "E676", Message: "not found", Cause: "E541"}
}

type sink struct{ values int }

func (s *sink) Register([]byte) error { s.values++; return nil }

func laneRefs(t *testing.T) []string {
	t.Helper()
	if _, err := secrets.ParseAll(SecretRefs); err != nil {
		t.Fatalf("the lane's secret refs do not parse: %v", err)
	}
	return SecretRefs
}

func source(mode secrets.Mode, personalNode bool) (*secrets.Source, *absentResolver, *sink, *int) {
	res, reg, audits := &absentResolver{}, &sink{}, new(int)
	env := map[string]string{"ANDROID_KEYSTORE_BASE64": "ZmFrZQ==", "ANDROID_KEYSTORE_PASSWORD": "fake-store-pw", "ANDROID_KEY_ALIAS": "fake-alias", "ANDROID_KEY_PASSWORD": "fake-key-pw"}
	facts := secrets.Facts{
		Job:    trust.JobFacts{Trust: "internal", Release: true, PersonalOwnRun: true, Mode: trust.Personal},
		Runner: trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: personalNode, Dedicated: true, Isolation: "vm", UIDSeparation: true},
	}
	return secrets.New(secrets.Config{Mode: mode, Attempt: "a1", Resolver: res, Registry: reg, RedactorInstalled: func() bool { return true }, Facts: facts,
		NodeID: "node-2", NodeEligible: true, AgeKeyPath: "/nonexistent/age.key", Legacy: &secrets.LegacyEnv{Getenv: func(k string) string { return env[k] }},
		Audit: func(secrets.Event) error { *audits++; return nil }}), res, reg, audits
}

func TestOperatorNodeGetsTheKeystoreThroughTheCompatSource(t *testing.T) {
	s, _, reg, audits := source(secrets.ModePersonalLocal, true)
	vals, err := s.Resolve(context.Background(), laneRefs(t))
	if err != nil || len(vals) != 4 || reg.values != 4 || *audits != 4 {
		t.Fatalf("personal node: %d values, %d registered, %d audits, %v", len(vals), reg.values, *audits, err)
	}
	if vals["ANDROID_KEY_ALIAS"] != "fake-alias" {
		t.Fatalf("env names %v", vals)
	}
}

func TestAgentNodeGetsNoCompatSource(t *testing.T) {
	s, res, reg, audits := source(secrets.ModeAgentNode, false)
	vals, err := s.Resolve(context.Background(), laneRefs(t))
	if err == nil || len(vals) != 0 || reg.values != 0 || *audits != 0 {
		t.Fatalf("an agent node got %d values, %d audits (err %v)", len(vals), *audits, err)
	}
	if res.calls == 0 {
		t.Fatal("the node was refused before the resolver was asked: the test no longer proves the compat source is what is denied")
	}
}

func TestTeamCoordinatorResolvesNothing(t *testing.T) {
	s, res, _, _ := source(secrets.ModeTeamCoordinator, false)
	if vals, err := s.Resolve(context.Background(), laneRefs(t)); err == nil || len(vals) != 0 || res.calls != 0 {
		t.Fatalf("a team coordinator resolved %d values with %d resolver calls (err %v)", len(vals), res.calls, err)
	}
	if refs, err := s.Refs(laneRefs(t)); err != nil || len(refs) != 4 || refs[0].Class != model.SecretClass("release") {
		t.Fatalf("lease refs %+v %v", refs, err)
	}
}
