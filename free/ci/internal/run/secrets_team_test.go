package run

import (
	"context"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestSubmittedSecretSourceNeverResolvesOnCoordinator(t *testing.T) {
	for _, tc := range []struct {
		name, ref string
		team      bool
	}{
		{"personal", "project/FOO", false},
		{"submitted-project", "project/FOO", true},
		{"submitted-release", "release/FOO", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := &fakeCli{}
			ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli, Facts: localFacts})
			t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
			a := AttemptInfo{SourceClass: "owner", AttemptID: "a", Job: model.Job{Kind: "release"},
				SecretsInfo: SecretsInfo{TeamRun: tc.team, ReleaseAuthorized: true}}
			source := newSecretSource(a, func() ciexec.Redactor { return Redactions.NewFilter() })
			defer source.(*secrets.Source).Zero()
			_, err := source.Resolve(context.Background(), []string{tc.ref})
			if tc.team {
				if secrets.CodeOf(err) != "E677" || cli.calls != 0 {
					t.Fatalf("submitted source resolved: err=%v calls=%d", err, cli.calls)
				}
			} else if err != nil || cli.calls != 2 {
				t.Fatalf("personal source failed: err=%v calls=%d", err, cli.calls)
			}
		})
	}
}

func TestTeamSecretFactsRequireReleaseAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       model.JobKind
		authorized bool
		want       bool
	}{
		{"ordinary", "test", false, false},
		{"ordinary-authorized", "test", true, false},
		{"release-denied", "release", false, false},
		{"release-authorized", "release", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := &runState{source: "internal", c: &Coordinator{Opts: Options{TeamRun: true,
				ReleaseAuthorized: tc.authorized}}, sess: &Session{}}
			a := rs.info(plan.PlannedJob{}, model.Job{Kind: tc.kind}, "a", 1)
			f := teamFacts(a)
			if !a.TeamRun || f.Job.Mode != trust.Team || f.Job.Release != tc.want ||
				f.Job.PersonalOwnRun || f.Runner.PersonalOwnNode || f.Runner.Dedicated || f.Job.Trust != "internal" {
				t.Fatalf("incorrect submitted facts: attempt=%+v facts=%+v", a.SecretsInfo, f)
			}
			refs, err := secrets.ParseAll([]string{"release/FOO"})
			if err != nil {
				t.Fatal(err)
			}
			if len(secrets.Check(refs, f)) == 0 {
				t.Fatal("team coordinator inherited the personal release exception")
			}
		})
	}
}
