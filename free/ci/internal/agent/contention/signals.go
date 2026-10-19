// Package contention is the node-side contention layer of Epic D10 (P7-NODE-13): it keeps a
// developer's machine usable while CI runs on it.
//
// Purpose: gate new leases on battery, interactive use and the operator's reservation, run leased
// jobs at background priority, and report the availability facts the scheduler reads.
// Inputs:  the P7-CI-32 capacity probe (exec.Probe), the node's settings, the host slot table
//
//	(internal/agent/slots), an optional injected activity signal.
//
// Outputs: Decision (allowed or the node.* reason), a priority-wrapping exec.Isolator, availability
//
//	facts for the capability snapshot, the persisted local policy file.
//
// Constraints: one probe (CI D6): no second battery or capacity probe lives here, and exec.Admit stays
//          the admission of local runs while this gate governs remote and background leases from the
//          node's explicit settings (a stored reservation holds whether or not a terminal is
//          attached; only CI=true switches it off); no root is
//
//	needed, what cannot be applied is reported unknown and never guessed; the operator's own
//	foreground run is never gated (floor of one job, X17). The files in this package never
//	import os/exec: priority is an argv prefix the executor runs.
package contention

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Interactive values of availability.interactive.
const (
	Active  = "active"
	Idle    = "idle"
	Unknown = "unknown"
)

// Signals is what the host looks like right now, read from one capacity snapshot.
type Signals struct {
	// BatteryKnown is true when the probe found a battery.
	BatteryKnown bool
	// OnBattery is true only for a known battery that is not on mains power.
	OnBattery bool
	// Interactive is Active, Idle or Unknown. The probe reports only whether a terminal is attached,
	// so without an injected signal it yields Active or Unknown, never Idle (a daemon without a
	// terminal says nothing about the person at the keyboard).
	Interactive string
}

// ReadSignals derives the signals from c; a non-nil injected signal replaces the interactive value
// (tests, and a later activity source), and a non-empty Interactive there wins.
func ReadSignals(c exec.Capacity, injected *Signals) Signals {
	s := Signals{BatteryKnown: c.BatteryPercent >= 0, Interactive: Unknown}
	s.OnBattery = s.BatteryKnown && !c.PluggedIn
	if c.Interactive {
		s.Interactive = Active
	}
	if injected != nil {
		if injected.Interactive != "" {
			s.Interactive = injected.Interactive
		}
		if injected.BatteryKnown {
			s.BatteryKnown, s.OnBattery = true, injected.OnBattery
		}
	}
	return s
}

func fact[T any](v *T, source string, now time.Time) model.Fact[T] {
	if v == nil {
		return model.Fact[T]{Source: source, ObservedAt: now, Confidence: "unknown"}
	}
	return model.Fact[T]{Value: v, Source: source, ObservedAt: now, Confidence: "known"}
}

// ApplyFacts writes the facts this layer owns into an agent-built capability document: battery
// percentage, plugged-in, interactive, the slot limit and the running count. Anything the probe could
// not read stays an explicit unknown. State and the assigned policy fields are left as they are.
func ApplyFacts(c *model.Capability, cap exec.Capacity, sig Signals, slotLimit, running int, now time.Time) {
	var pct *int
	if cap.BatteryPercent >= 0 {
		pct = &cap.BatteryPercent
	}
	var plugged *bool
	switch {
	case cap.PluggedIn:
		v := true
		plugged = &v
	case sig.BatteryKnown:
		v := false
		plugged = &v
	}
	var inter *string
	if sig.Interactive != "" && sig.Interactive != Unknown {
		inter = &sig.Interactive
	}
	a := &c.Availability
	a.BatteryPct, a.PluggedIn, a.Interactive = fact(pct, "probe", now), fact(plugged, "probe", now), fact(inter, "probe", now)
	a.Slots, a.Running = fact(&slotLimit, "self", now), fact(&running, "self", now)
}
