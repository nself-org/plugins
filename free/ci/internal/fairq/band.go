package fairq

// Purpose: the priority bands of the fair queue (Epic D9) and the job priority
// ranks, derived from the trigger and the foreground flag.
// Inputs:  a declared band name, a trigger name, the foreground flag.
// Outputs: a Band; its name; a priority rank.
// Constraints: pure; strict band order is interactive, normal, background; an
// unknown name is never a band, so a typo cannot jump the queue.

// Band is a priority band; a lower value is served first.
type Band int

// The bands, highest first.
const (
	Interactive Band = iota
	Normal
	Background
	bandCount
)

var bandNames = [bandCount]string{"interactive", "normal", "background"}

// String returns the band name used in plans, decisions and `ci runs queue`.
func (b Band) String() string {
	if b < 0 || b >= bandCount {
		return "normal"
	}
	return bandNames[b]
}

// Parse returns the band named s.
func Parse(s string) (Band, bool) {
	for i, n := range bandNames {
		if n == s {
			return Band(i), true
		}
	}
	return Normal, false
}

// For derives the band of a pipeline: a declared band wins; otherwise a
// foreground run, an operator's own run (trigger manual, local or none) is
// interactive, a schedule is background and everything else (push, pull_request,
// tag, api) is normal. Drill, flaky-confirmation and nightly jobs are meant to
// declare `background` themselves (P7-SCHED-12 wires that).
func For(declared, trigger string, foreground bool) Band {
	if b, ok := Parse(declared); ok {
		return b
	}
	switch {
	case foreground, trigger == "", trigger == "manual", trigger == "local":
		return Interactive
	case trigger == "schedule":
		return Background
	}
	return Normal
}

// rank orders job priorities within a project: high first, an unknown value is normal.
func rank(priority string) int {
	switch priority {
	case "high":
		return 0
	case "low":
		return 2
	}
	return 1
}
