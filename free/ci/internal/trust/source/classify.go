package source

// Purpose: Classify turns normalized trigger facts (contract:ci.provider, P7-HOST-01) into a
// source trust class (Epic D17): trust = min(actor permission, revision provenance), then
// ceilings for auth, forks, bots, lockfile-only diffs and a moved head.
// Inputs:  model.TriggerFacts (already re-fetched by the hook for forge sources).
// Outputs: Result{Class, Reasons, RequiresApproval, VisibilityZone}; ApplyCap for the machine cap.
// Constraints: pure and deterministic. A fact RAISES trust only when its source is the forge API or
//          this machine (Authoritative); a payload or caller claim can only lower it, so a forged
//          payload never gets above what the API facts give. A missing fact takes its untrusted
//          value. Reasons come in rule order and name only rules that bound the class.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const (
	owner        = model.TrustClass("owner")
	collaborator = model.TrustClass("collaborator")
	untrusted    = model.TrustClass("untrusted")
)

// ReasonVerified is the only reason of an owner result: no rule bound it.
const ReasonVerified = "source.verified"

// Classify decides the trust class of one trigger.
func Classify(f model.TriggerFacts) Result {
	r := Result{Class: owner, VisibilityZone: zone(f)}
	bind := func(ceiling model.TrustClass, reason string) {
		if trust.RankTrust(ceiling) < trust.RankTrust(owner) {
			r.Reasons = append(r.Reasons, reason)
		}
		if trust.RankTrust(ceiling) < trust.RankTrust(r.Class) {
			r.Class = ceiling
		}
	}
	if f.Auth != "per-repo" {
		bind(untrusted, ReasonAuthNotPerRepo)
	}
	bind(actorClass(f), actorReason(f))
	if repoMismatch(f) {
		bind(untrusted, ReasonFork)
	}
	provenance(f, bind)
	if bot(f) {
		bind(untrusted, ReasonBot)
	}
	if files, ok := claimed(f.ChangedFiles); ok && LockfileOnly(files) {
		bind(untrusted, ReasonLockfileOnly)
	}
	if cur, ok := claimed(f.HeadCurrent); ok && !cur {
		bind(untrusted, ReasonHeadChanged)
	}
	if len(r.Reasons) == 0 {
		r.Reasons = []string{ReasonVerified}
	}
	r.RequiresApproval = needsApproval(f, r.Class)
	return r
}

// actorClass is the class the actor's repository permission earns. A schedule has no actor:
// the repository's own configuration started it.
func actorClass(f model.TriggerFacts) model.TrustClass {
	if f.Kind == "schedule" {
		return owner
	}
	perm, ok := trusted(f.ActorPermission)
	if !ok {
		return untrusted
	}
	switch strings.ToLower(perm) {
	case "admin", "owner":
		return owner
	case "maintain", "write", "push":
		return collaborator
	}
	return untrusted
}

func actorReason(f model.TriggerFacts) string {
	if _, ok := trusted(f.ActorPermission); !ok && f.Kind != "schedule" {
		return ReasonActorUnknown
	}
	return ReasonActor
}

// repoMismatch: the change comes from another repository, or no authoritative fact says it does not.
func repoMismatch(f model.TriggerFacts) bool {
	if fork, ok := claimed(f.Fork); ok && fork {
		return true
	}
	if head, ok := claimed(f.HeadRepo); ok && !strings.EqualFold(head, f.Repo) {
		return true
	}
	if f.Kind != "pull_request" && f.Kind != "runner_demand" {
		return false
	}
	fork, fok := trusted(f.Fork)
	head, hok := trusted(f.HeadRepo)
	return !((fok && !fork) || (hok && strings.EqualFold(head, f.Repo)))
}

// provenance binds the class to where the revision came from.
func provenance(f model.TriggerFacts, bind func(model.TrustClass, string)) {
	reachable, rok := trusted(f.SHAReachable)
	reachable = rok && reachable
	protected, pok := trusted(f.ProtectedRef)
	protected = pok && protected
	switch f.Kind {
	case "push", "schedule":
		if !protected && !reachable {
			bind(collaborator, ReasonProvenance)
		}
	case "pull_request":
		if !reachable {
			bind(collaborator, ReasonProvenance)
		}
	case "tag", "api", "manual":
		if !reachable {
			bind(untrusted, ReasonUnreachable)
		}
	case "runner_demand":
		runnerDemand(f, bind)
		if !protected && !reachable {
			bind(collaborator, ReasonProvenance)
		}
	default:
		bind(untrusted, ReasonProvenance)
	}
}

// runnerDemand: a job a forge asks a runner for is classified by the other facts only when the
// group takes protected refs only, the event is one that runs the repository's own workflow
// definition, and the head is the repository itself (HOST delta 10, review R5).
func runnerDemand(f model.TriggerFacts, bind func(model.TrustClass, string)) {
	only, ok := trusted(f.GroupProtectedOnly)
	event, eok := trusted(f.RunEvent)
	head, hok := trusted(f.HeadRepo)
	switch {
	case !ok || !only, !eok, !hok, !strings.EqualFold(head, f.Repo):
		bind(untrusted, ReasonRunnerDemand)
	case event == "pull_request_target" || event == "workflow_run":
		bind(untrusted, ReasonRunnerDemand)
	}
}

func bot(f model.TriggerFacts) bool {
	if b, ok := claimed(f.Bot); ok && b {
		return true
	}
	login, ok := claimed(f.Actor)
	return ok && IsBot(login)
}

// zone is the privacy zone the repository's visibility allows: only a public repository, said so
// by the API, may leave the private infrastructure. Unknown or changed visibility stays private.
func zone(f model.TriggerFacts) model.PrivacyZone {
	if v, ok := trusted(f.Visibility); ok && strings.EqualFold(v, "public") {
		return "hosted-allowed"
	}
	return "private-infrastructure"
}

func needsApproval(f model.TriggerFacts, class model.TrustClass) bool {
	if first, ok := claimed(f.FirstTime); ok && first {
		return true
	}
	if _, ok := trusted(f.FirstTime); ok {
		return false
	}
	return class != owner
}

// ApplyCap lowers r to cap (the `trust.machine_invoked_cap` of a machine-invoked run, Epic D27)
// and records ReasonMachineCap when it did. An unknown cap value decodes to untrusted.
func ApplyCap(r Result, cap model.TrustClass) Result {
	c := trust.DecodeTrust(string(cap))
	if trust.RankTrust(c) >= trust.RankTrust(r.Class) {
		return r
	}
	r.Class = c
	r.Reasons = append(append([]string(nil), r.Reasons...), ReasonMachineCap)
	return r
}
