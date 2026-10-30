package main

// Purpose: the generated tables of Scheduling-and-Cost.md: the placement
// presets and the placement reason codes.
// Inputs:  sched.Presets (the preset table in code); the reason codes read
// from the source of free/ci (reasonCodes), so a new constant shows up the
// next time the generator runs.
// Outputs: two Markdown tables, each spliced into its region of the page.
// Constraints: values are rendered from code only; the component and class
// names below must stay in the order of sched.Preset (a test checks the
// counts).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// schedulingPage is the page that holds the scheduling regions.
const schedulingPage = ".github/wiki/guides/ci/Scheduling-and-Cost.md"

// presetComponents names the score weights of a preset, in sched.Preset order.
var presetComponents = [9]string{"wait", "runtime", "cold_start", "transfer", "cache", "contention", "cost", "scarcity", "reliability"}

// presetClasses names the class bias entries of a preset, in sched.Preset order.
var presetClasses = [5]string{"local", "owned", "included", "metered", "ephemeral"}

// presetTable renders every preset: weights per mille, the milliseconds one
// micro-unit of cost is worth, and the per-class bias in milliseconds.
func presetTable() string {
	presets := sched.Presets()
	names := make([]string, 0, len(presets))
	for n := range presets {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("| Preset |")
	for _, c := range presetComponents {
		fmt.Fprintf(&b, " `%s` |", c)
	}
	b.WriteString(" ms per cost micro-unit |")
	for _, c := range presetClasses {
		fmt.Fprintf(&b, " bias `%s` (ms) |", c)
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(presetComponents)+1+len(presetClasses)) + "\n")
	for _, n := range names {
		p := presets[n]
		fmt.Fprintf(&b, "| `%s` |", n)
		for _, w := range p.Weights {
			fmt.Fprintf(&b, " %d |", w)
		}
		fmt.Fprintf(&b, " %d |", p.MsPerCostMicro)
		for _, c := range p.ClassBiasMs {
			fmt.Fprintf(&b, " %d |", c)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// placementReasonTable renders every model.PlacementReason with whether it
// is static (a runner cannot become eligible without a configuration change).
func placementReasonTable(reasons []ReasonCode) string {
	var b strings.Builder
	b.WriteString("| Reason | Clears without a configuration change |\n|---|---|\n")
	for _, r := range reasons {
		if r.Type != "PlacementReason" {
			continue
		}
		clears := "yes"
		if model.PlacementReason(r.Value).Static() {
			clears = "no (static)"
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", r.Value, clears)
	}
	return b.String()
}
