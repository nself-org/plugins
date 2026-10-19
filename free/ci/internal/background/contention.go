package background

// Purpose: the contention predicate of background CI (Epic D14, NODE D10): a pure
// function from a runner's capability snapshot to "may a background job start
// here now". It reads only the availability facts of contract:ci.runner-capability.
// Inputs:  the sched.Runner of a placement (capability, declaration, Active flag).
// Outputs: a Verdict listing every contention cause, empty when the node is idle.
// Constraints: pure, no clock, no I/O. A missing (unknown) fact is never idle on
// an operator-owned node: unplugged-or-unknown power, and interactive state
// other than a known "idle", both refuse there. On a node the operator does not
// own (hosted, ephemeral, a teammate's), only a known fact refuses, because the
// operator has no interactive use to protect. The rules mirror
// sched/eligibility.go so a stale placement and the admission re-check agree.

import (
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// Cause names one reason a node is busy for background work.
type Cause string

// The contention causes; every one maps to the background.contention reason.
const (
	OnBattery          Cause = "on battery or not plugged in"
	PowerUnknown       Cause = "power state unknown on an operator-owned node"
	InteractiveActive  Cause = "interactive use is active"
	InteractiveUnknown Cause = "interactive state unknown on an operator-owned node"
	RunnerActive       Cause = "runner reports active use"
)

// Verdict is the contention state of one runner.
type Verdict struct {
	Causes []Cause
}

// Busy reports whether any cause holds.
func (v Verdict) Busy() bool { return len(v.Causes) > 0 }

// OperatorOwned reports whether r is a machine the operator owns, by its trust
// declaration or its capability's ownership fact.
func OperatorOwned(r sched.Runner) bool {
	own := r.Capability.Identity.Ownership.Value
	return r.Decl.Ownership == "operator" || own != nil && *own == "operator"
}

// Contended evaluates the availability facts of r.
func Contended(r sched.Runner) Verdict {
	var v Verdict
	a := r.Capability.Availability
	owned := OperatorOwned(r)
	switch plugged := a.PluggedIn.Value; {
	case plugged != nil && !*plugged:
		v.Causes = append(v.Causes, OnBattery)
	case plugged == nil && owned:
		v.Causes = append(v.Causes, PowerUnknown)
	}
	switch state := a.Interactive.Value; {
	case state != nil && *state == "active":
		v.Causes = append(v.Causes, InteractiveActive)
	case owned && (state == nil || *state != "idle"):
		v.Causes = append(v.Causes, InteractiveUnknown)
	}
	if r.Active {
		v.Causes = append(v.Causes, RunnerActive)
	}
	return v
}
