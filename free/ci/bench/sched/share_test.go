package sched

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/bench"
)

func shareResult(p95 float64, unit string) bench.Result {
	return bench.NewResult("sha", 20, []bench.Metric{{Name: MetricShare, Unit: unit, P50: p95 / 2, P95: p95, Max: p95, N: 20}})
}

func TestShareBudgetIsWorstP95TimesHeadroom(t *testing.T) {
	b, err := ShareFromBaseline([]bench.Result{shareResult(1.0, "pct"), shareResult(2.0, "pct")}, "n")
	if err != nil {
		t.Fatal(err)
	}
	if b.P95Pct != 2.5 {
		t.Fatalf("ceiling %v, want 2.5", b.P95Pct)
	}
	if v := CheckShare(b, shareResult(2.5, "pct")); v != "" {
		t.Fatalf("a result at the ceiling was refused: %s", v)
	}
	if v := CheckShare(b, shareResult(2.6, "pct")); !strings.Contains(v, "exceeds") {
		t.Fatalf("a result above the ceiling passed: %q", v)
	}
}

func TestShareCheckFailsClosed(t *testing.T) {
	b := ShareBudget{Schema: ShareSchema, P95Pct: 5}
	if v := CheckShare(b, bench.NewResult("s", 1, nil)); !strings.Contains(v, "missing") {
		t.Fatalf("missing metric passed: %q", v)
	}
	if v := CheckShare(b, shareResult(1, "ms")); !strings.Contains(v, "unit") {
		t.Fatalf("wrong unit passed: %q", v)
	}
	if _, err := LoadShare(filepath.Join(t.TempDir(), "none.json")); err == nil {
		t.Fatal("a missing ceiling file loaded")
	}
}

func TestDecideAdmitsOneLocalJob(t *testing.T) {
	d, err := Decide(context.Background(), filepath.Join(t.TempDir(), "d"))
	if err != nil {
		t.Fatal(err)
	}
	if d.PlaceMS <= 0 || d.AdmitMS < d.PlaceMS {
		t.Fatalf("implausible decision timing %+v", d)
	}
}
