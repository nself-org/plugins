package planview

// Purpose: the human lines of `ci plan` (one placement cell per job) and of
// `ci explain` (the chosen runner and every alternative).
// Inputs: a model.PlacementPlan. Outputs: strings; no colour, no timestamps.
// Constraints: secret classes by name only; an unknown value prints as
// "unknown", never as 0; alternatives print in the document's order (runner id).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Cells returns one PLACEMENT cell per job, in job order.
func Cells(p model.PlacementPlan) []string {
	out := make([]string, len(p.Jobs))
	for i, j := range p.Jobs {
		out[i] = cell(j)
	}
	return out
}

func cell(j model.JobPlacement) string {
	if j.Placement != nil {
		pl := j.Placement
		return fmt.Sprintf("%s (%s, %s) cost %s", pl.RunnerID, pl.Location, pl.CostClass, costText(pl.Estimate))
	}
	codes := codesOf(j.Reasons)
	if len(codes) == 0 {
		codes = alternativeCodes(j.Alternatives, 3)
	}
	if len(codes) == 0 {
		return j.Status
	}
	return j.Status + ": " + strings.Join(codes, ",")
}

func costText(e model.Estimate) string {
	if e.CostMicro == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d micro [%s]", *e.CostMicro, e.CostConfidence)
}

func codesOf(rs []model.PlacementReasonDetail) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Code
	}
	return out
}

// alternativeCodes lists the most common reason codes across the
// alternatives, most frequent first, at most n.
func alternativeCodes(alts []model.Alternative, n int) []string {
	count := map[string]int{}
	for _, a := range alts {
		for _, r := range a.Reasons {
			count[r.Code]++
		}
	}
	codes := make([]string, 0, len(count))
	for c := range count {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		if count[codes[i]] != count[codes[j]] {
			return count[codes[i]] > count[codes[j]]
		}
		return codes[i] < codes[j]
	})
	if len(codes) > n {
		codes = codes[:n]
	}
	return codes
}

// componentOrder is the scoring order of Epic SCHED D4.
var componentOrder = []string{"wait", "runtime", "cold_start", "transfer", "cache", "contention", "cost", "scarcity", "reliability"}

func componentText(c map[string]model.Component) string {
	parts := make([]string, 0, len(componentOrder))
	for _, n := range componentOrder {
		if v, ok := c[n]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d(%s)", n, v.ValueMs, v.Confidence))
		}
	}
	return strings.Join(parts, " ")
}

// ExplainLines renders every job of the document: the status, the chosen
// runner with its score components, and each alternative with the reason
// codes that rule it out or the components that score it.
func ExplainLines(p model.PlacementPlan, source string) []string {
	var out []string
	for _, j := range p.Jobs {
		head := fmt.Sprintf("job %s", j.ID)
		if j.MatrixKey != "" {
			head += " [" + j.MatrixKey + "]"
		}
		out = append(out, fmt.Sprintf("%s  %s  band %s  source %s", head, j.Status, j.Band, source))
		out = append(out, fmt.Sprintf("  needs  platform %s  cpu %d  mem_mb %d  isolation_min %s  network %s  secret classes %s  tools %s",
			j.Requirements.Platform, j.Requirements.CPU, j.Requirements.MemMB, orNone(string(j.Requirements.IsolationMin)),
			orNone(string(j.Requirements.Network)), orNone(classNames(j.SecretClasses)), orNone(strings.Join(j.Requirements.Tools, ","))))
		for _, r := range j.Reasons {
			out = append(out, fmt.Sprintf("  reason  %s  %s", r.Code, r.Detail))
		}
		if pl := j.Placement; pl != nil {
			out = append(out, fmt.Sprintf("  chosen  %s  provider %s  %s  %s  score %d  cost %s", pl.RunnerID, orNone(pl.Provider), pl.Location, pl.CostClass, pl.Score, costText(pl.Estimate)))
			out = append(out, "    "+componentText(pl.Components))
		}
		chosen := ""
		if j.Placement != nil {
			chosen = j.Placement.RunnerID
		}
		for _, a := range j.Alternatives {
			out = append(out, alternativeLine(a, a.RunnerID == chosen))
			if a.Eligible && a.Components != nil && a.RunnerID != chosen {
				out = append(out, "    "+componentText(a.Components))
			}
		}
	}
	return out
}

func alternativeLine(a model.Alternative, chosen bool) string {
	if chosen {
		return fmt.Sprintf("  alt     %s  chosen", a.RunnerID)
	}
	if !a.Eligible {
		return fmt.Sprintf("  alt     %s  ineligible  %s", a.RunnerID, strings.Join(codesOf(a.Reasons), ","))
	}
	if a.Score == nil {
		return fmt.Sprintf("  alt     %s  eligible  score unknown", a.RunnerID)
	}
	return fmt.Sprintf("  alt     %s  eligible  score %d", a.RunnerID, *a.Score)
}

func classNames(cs []model.SecretClass) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = string(c)
	}
	return strings.Join(names, ",")
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
