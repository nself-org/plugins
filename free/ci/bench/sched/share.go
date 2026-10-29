package sched

// Purpose: the ceiling for sched.share_tiny_pct. perfbench-budget/v1 only holds
// millisecond ceilings (`perfbench check` refuses any other unit), so the share
// has its own small document, derived and checked the same way.
// Inputs:  baseline results of one platform; a result to check.
// Outputs: a ShareBudget; the violations of a result against it.
// Constraints: the ceiling is the worst baseline p95 times 1.25, rounded up to
// 0.1 percentage point; a result without the metric fails closed.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/nself-org/plugins/free/ci/bench"
)

// ShareSchema is the schema field of a share budget.
const ShareSchema = "schedbench-share-budget/v1"

// ShareBudget is the ceiling document.
type ShareBudget struct {
	Note     string  `json:"_note"`
	Schema   string  `json:"schema"`
	Platform string  `json:"platform"`
	P95Pct   float64 `json:"p95_pct"`
}

// ShareFromBaseline derives the ceiling from the baseline runs of one platform.
func ShareFromBaseline(runs []bench.Result, note string) (ShareBudget, error) {
	worst, err := bench.WorstOf(runs)
	if err != nil {
		return ShareBudget{}, err
	}
	m, ok := worst.Find(MetricShare)
	if !ok || m.P95 <= 0 || math.IsNaN(m.P95) {
		return ShareBudget{}, fmt.Errorf("baseline has no usable %s", MetricShare)
	}
	return ShareBudget{Note: note, Schema: ShareSchema, Platform: fmt.Sprintf("%s/%s", worst.GOOS, worst.GOARCH),
		P95Pct: math.Ceil(m.P95*bench.Headroom*10) / 10}, nil
}

// LoadShare reads a share budget.
func LoadShare(path string) (ShareBudget, error) {
	var b ShareBudget
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err = json.Unmarshal(raw, &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if b.Schema != ShareSchema || b.P95Pct <= 0 {
		return b, fmt.Errorf("%s: not a %s document", path, ShareSchema)
	}
	return b, nil
}

// CheckShare returns the violation of res against b, "" when it holds.
func CheckShare(b ShareBudget, res bench.Result) string {
	m, ok := res.Find(MetricShare)
	switch {
	case !ok:
		return MetricShare + ": missing from the result"
	case m.Unit != "pct":
		return fmt.Sprintf("%s: unit %q, want pct", MetricShare, m.Unit)
	case math.IsNaN(m.P95) || m.P95 <= 0:
		return fmt.Sprintf("%s: p95 %v is not a positive number", MetricShare, m.P95)
	case m.P95 > b.P95Pct:
		return fmt.Sprintf("%s: p95 %.2f%% exceeds the ceiling %.1f%%", MetricShare, m.P95, b.P95Pct)
	}
	return ""
}
