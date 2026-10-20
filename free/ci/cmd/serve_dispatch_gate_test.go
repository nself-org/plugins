package main

// Purpose: D-1337. The dispatch of a queued run must not start a release-kind job or a secret reference
//          for a submitter who lacks release.start, and a team coordinator must hold no PERSONAL secret
//          facts. Inputs: a real store and roles Service, plans built in code. No network, no git.

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func gatePlan(jobs ...model.Job) *plan.Plan {
	p := &plan.Plan{}
	for i, j := range jobs {
		p.Jobs = append(p.Jobs, plan.PlannedJob{ID: string(rune('a' + i)), Job: j})
	}
	return p
}

func TestSubmitterGateRefusesReleaseWorkForAMember(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	op := svc.Operator("cli:alice")
	for p, r := range map[string]roles.Role{"bob": roles.Member, "carol": roles.Release} {
		if err = svc.Grant(ctx, op, roles.Request{Principal: p, Role: r, Projects: []string{roles.AllProjects}}); err != nil {
			t.Fatal(err)
		}
	}
	plain := gatePlan(model.Job{Kind: "test"})
	rel := gatePlan(model.Job{Kind: "test"}, model.Job{Kind: "release"})
	sec := gatePlan(model.Job{Kind: "build", Secrets: []string{"project/KEY"}})
	cases := []struct {
		name    string
		by      string
		p       *plan.Plan
		svc     *roles.Service
		refused bool
	}{
		{"member, plain job", "user:bob role=member", plain, svc, false},
		{"member, release job", "user:bob role=member", rel, svc, true},
		{"member, secret ref", "user:bob role=member", sec, svc, true},
		{"release submitter, release job", "user:carol role=release", rel, svc, false},
		{"operator, secret ref", "operator:cli:alice role=admin", sec, svc, false},
		{"claimed role is not trusted", "user:bob role=release", rel, svc, true},
		{"no roles service, release job", "user:carol role=release", rel, nil, true},
		{"webhook (no submitter), release job", "", rel, svc, false},
	}
	for _, c := range cases {
		err := submitterGate(ctx, c.svc, pipelineFacts{Submitter: c.by}, "o/r", c.p)
		if (err != nil) != c.refused {
			t.Errorf("%s: refused=%v err=%v", c.name, err != nil, err)
		}
	}
}

func TestTeamServeConfiguresRefsOnlySecrets(t *testing.T) {
	ctx := context.Background()
	st := modeStore(t)
	fs, _, _ := newServeV15Flags()
	if err := fs.Parse([]string{"--team"}); err != nil {
		t.Fatal(err)
	}
	end, err := serveModeStart(ctx, fs, st, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	src := cirun.Registered.NewSecrets(cirun.AttemptInfo{AttemptID: "att-1", Home: t.TempDir()}, nil)
	_, err = src.Resolve(ctx, []string{"project/KEY"})
	if err == nil || !strings.Contains(err.Error(), secrets.ReasonRefsOnly) {
		t.Errorf("a team coordinator did not refuse with %s: %v", secrets.ReasonRefsOnly, err)
	}
	end()
	if cirun.SecretsMode() != secrets.ModePersonalLocal {
		t.Errorf("the secrets mode was not restored when the serve ended: %s", cirun.SecretsMode())
	}
}
