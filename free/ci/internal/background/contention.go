package background

// Purpose: the contention predicate of background CI (Epic D14, NODE D10): a pure
// function from a runner's capability snapshot to "may a background job start
// here now". It reads only the availability facts of contract:ci.runner-capability.
// Inputs:  the sched.Runner of a placement (capability, declaration, Active flag).
// Outputs: a Verdict listing every contention cause, empty when the node is idle.
// Constraints: pure, no clock, no I/O. A missing (unknown) fact is never idle on
// an operator-owned node: unplugged-or-unknown power, and interactive state
// other than a known "idle", both refuse there. A node counts as operator-owned
// unless it is hosted or ephemeral, or every ownership source that speaks names
// someone else: a paired laptop reports no ownership today, and treating that as
// "not the operator's" would run background work on a machine in use. On a node
// the operator does not own, only a known fact refuses, because the operator
// has no interactive use to protect. The signals are the ones D14 names
// (power and interactive state). Runner.Active is deliberately not a signal: the
// world sets it from "node is online", so refusing on it would refuse every
// online runner (see the P7-SCHED-12 handoff). Where sched/eligibility.go is
// narrower (unknown ownership counts as not owned), this predicate refuses more.

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
)

// Verdict is the contention state of one runner.
type Verdict struct {
	Causes []Cause
}

// Busy reports whether any cause holds.
func (v Verdict) Busy() bool { return len(v.Causes) > 0 }

// OperatorOwned reports whether r is a machine the operator may be using: not
// hosted or ephemeral, and either named "operator" by its trust declaration or
// capability ownership fact, or named by neither (unknown fails closed).
func OperatorOwned(r sched.Runner) bool {
	if kind := r.Capability.Location.Kind.Value; r.Location == "hosted" || r.Ephemeral || r.Decl.Hosted || kind != nil && *kind == "hosted" {
		return false
	}
	sources := []string{r.Decl.Ownership}
	if own := r.Capability.Identity.Ownership.Value; own != nil {
		sources = append(sources, *own)
	}
	foreign := false
	for _, v := range sources {
		switch v {
		case "operator":
			return true
		case "":
		default:
			foreign = true
		}
	}
	return !foreign
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
	return v
}
