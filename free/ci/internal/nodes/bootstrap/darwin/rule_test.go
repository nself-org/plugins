package darwin

// Purpose: the user-mode macOS rule of ADR 0025 Amendment 2026-10-01 (Epic D20, invariant I17), as an
// executable table. A Mac added WITHOUT --system has no uid separation: it runs owner-trust work from
// its operator's PERSONAL coordinator only, holds no secret class, and a TEAM coordinator refuses it with
// the isolation.uid_separation reason (E657 at the lease gate). A Mac added WITH --system is separated
// (distinct job users behind the sudoers job-shim rules) and may join a TEAM coordinator and hold secrets.
// Inputs:  trust.Declaration values for the two kinds of Mac and a few jobs.
// Constraints: pure table test over internal/trust; no node, no network.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func macDecl(separated bool, mode trust.CoordinatorMode, accepts ...model.TrustClass) trust.Declaration {
	return trust.Declaration{Accepts: accepts, Isolation: "process", Network: "restricted", SecretClasses: []model.SecretClass{"none"},
		Ownership: "operator", PersonalOwnNode: true, UIDSeparation: separated, NoLANRoute: true, CoordinatorMode: mode}
}

func has(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

func TestUserModeMacKeepsTheI17Exception(t *testing.T) {
	owner := trust.JobFacts{Trust: "owner", Network: "restricted", PersonalOwnRun: true}
	personal := macDecl(false, trust.Personal, "owner")
	if rs := trust.Admissible(owner, personal, trust.Effective{}); has(rs, trust.IsolationUIDSeparation) {
		t.Fatalf("a user-mode Mac refused owner work from its PERSONAL coordinator: %v", rs)
	}
	team := macDecl(false, trust.Team, "owner")
	if rs := trust.Admissible(owner, team, trust.Effective{}); !has(rs, trust.IsolationUIDSeparation) {
		t.Fatalf("a TEAM coordinator must refuse a user-mode Mac with isolation.uid_separation, got %v", rs)
	}
	teamRun := owner
	teamRun.PersonalOwnRun = false
	if rs := trust.Admissible(teamRun, personal, trust.Effective{}); !has(rs, trust.IsolationUIDSeparation) {
		t.Fatalf("work that is not the operator's own run must be refused on a user-mode Mac: %v", rs)
	}
	internal := trust.JobFacts{Trust: "internal", Network: "restricted"}
	if rs := trust.Admissible(internal, macDecl(false, trust.Personal, "owner", "internal"), trust.Effective{}); !has(rs, trust.IsolationUIDSeparation) {
		t.Fatalf("internal-trust work must be refused on a user-mode Mac: %v", rs)
	}
}

func TestSystemMacJoinsTeamAndHoldsSecrets(t *testing.T) {
	d := macDecl(true, trust.Team, "owner", "internal")
	d.SecretClasses = []model.SecretClass{"none", "project"}
	job := trust.JobFacts{Trust: "internal", SecretClasses: []model.SecretClass{"project"}, Network: "restricted"}
	if rs := trust.Admissible(job, d, trust.Effective{}); has(rs, trust.IsolationUIDSeparation) {
		t.Fatalf("a --system Mac is separated, yet the TEAM coordinator refused it for uid_separation: %v", rs)
	}
}
