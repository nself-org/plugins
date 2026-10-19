package bench

// Purpose: the perf-budget.json document (perfbench-budget/v1) and its
// derivation from a recorded baseline.
// Inputs:  a baseline Result, a headroom factor, a metric filter.
// Outputs: a Budget whose absolute p95 ceilings are the baseline p95 plus the
// headroom, rounded up to a whole millisecond.
// Constraints: the field names are the ones `perfbench check` decodes with
// DisallowUnknownFields; a metric that is not in the baseline is never given a
// ceiling (an invented number is the one thing this Ticket must not ship).

import (
	"fmt"
	"math"
	"sort"
)

// BudgetSchemaV1 is the value of the "schema" field of the budget file.
const BudgetSchemaV1 = "perfbench-budget/v1"

// Headroom is the allowed growth over the baseline p95 (Ticket constraint: +25%).
const Headroom = 1.25

// Budget is the budget document. P95MS maps a metric name to its p95 ceiling in ms.
type Budget struct {
	Note     string             `json:"_note"`
	Schema   string             `json:"schema"`
	Platform string             `json:"platform"`
	P95MS    map[string]float64 `json:"p95_ms"`
}

// BudgetFromResult derives a budget from a baseline. keep selects the metrics
// that get a ceiling (nil keeps every metric measured in milliseconds).
func BudgetFromResult(base Result, note string, keep func(name string) bool) (Budget, error) {
	b := Budget{Note: note, Schema: BudgetSchemaV1,
		Platform: fmt.Sprintf("%s/%s", base.GOOS, base.GOARCH), P95MS: map[string]float64{}}
	for _, m := range base.Metrics {
		if m.Unit != "ms" || (keep != nil && !keep(m.Name)) {
			continue
		}
		if math.IsNaN(m.P95) || m.P95 <= 0 {
			return b, fmt.Errorf("metric %s: baseline p95 %v is not a usable timing", m.Name, m.P95)
		}
		b.P95MS[m.Name] = math.Ceil(m.P95 * Headroom)
	}
	if len(b.P95MS) == 0 {
		return b, fmt.Errorf("baseline has no metric to budget")
	}
	return b, nil
}

// Names lists the budgeted metric names, sorted.
func (b Budget) Names() []string {
	out := make([]string, 0, len(b.P95MS))
	for n := range b.P95MS {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
