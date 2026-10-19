package roles

// Purpose: the three roles (Epic D19: member < release < admin) and the action catalogue that maps each
// privileged action to its minimum role.
// Inputs:  an action name.
// Outputs: Role ordering, the catalogue, Needs(action).
// Constraints: exactly three roles, no per-user permissions (ADR 0025); an unknown action is refused
//          (fail closed), never defaulted; a view action is not audited, every other decision is.

import "sort"

// Role is a TEAM role.
type Role string

// The three roles, lowest first.
const (
	Member  Role = "member"
	Release Role = "release"
	Admin   Role = "admin"
)

// Rank orders roles; an unknown role ranks 0 and satisfies nothing.
func (r Role) Rank() int {
	switch r {
	case Member:
		return 1
	case Release:
		return 2
	case Admin:
		return 3
	}
	return 0
}

// ParseRole accepts only the three role names.
func ParseRole(s string) (Role, bool) {
	r := Role(s)
	return r, r.Rank() > 0
}

// Action names (Epic D19). Other Tickets call AuthorizeAction with these.
const (
	ActPolicyTeamSet     = "policy.team.set"
	ActRunnerRegister    = "runner.register"
	ActRunnerRevoke      = "runner.revoke"
	ActNodeTrustSet      = "node.trust.set"
	ActProviderAuthorize = "provider.authorize"
	ActPolicySet         = "policy.set"
	ActPrivacyZoneSet    = "privacy_zone.set"
	ActSecretAuthorize   = "secret.authorize"
	ActRunApprove        = "run.approve"
	ActSpendApprove      = "spend.approve"
	ActReleaseStart      = "release.start"
	ActLogsView          = "logs.view"
	ActArtifactsView     = "artifacts.view"
	ActAuditView         = "audit.view"
	ActRolesManage       = "roles.manage"
	ActTokenManage       = "token.manage"
)

type actionRule struct {
	min        Role
	audited    bool
	scoped     bool // the actor's project list applies; release.start also needs an allowlisted project
	needsAuthr bool // run approval by someone other than the run's author
}

var catalogue = map[string]actionRule{
	ActPolicyTeamSet:     {min: Admin, audited: true},
	ActRunnerRegister:    {min: Admin, audited: true},
	ActRunnerRevoke:      {min: Admin, audited: true},
	ActNodeTrustSet:      {min: Admin, audited: true},
	ActProviderAuthorize: {min: Admin, audited: true},
	ActPolicySet:         {min: Admin, audited: true},
	ActPrivacyZoneSet:    {min: Admin, audited: true},
	ActSecretAuthorize:   {min: Admin, audited: true},
	ActRunApprove:        {min: Release, audited: true, scoped: true, needsAuthr: true},
	ActSpendApprove:      {min: Admin, audited: true},
	ActReleaseStart:      {min: Release, audited: true, scoped: true},
	ActLogsView:          {min: Member, scoped: true},
	ActArtifactsView:     {min: Member, scoped: true},
	ActAuditView:         {min: Release, audited: true},
	ActRolesManage:       {min: Admin, audited: true},
	ActTokenManage:       {min: Admin, audited: true},
}

// Needs returns the minimum role of an action.
func Needs(action string) (Role, bool) {
	r, ok := catalogue[action]
	return r.min, ok
}

// Actions lists the catalogue, sorted.
func Actions() []string {
	out := make([]string, 0, len(catalogue))
	for a := range catalogue {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
