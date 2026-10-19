package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarizeNearestRank(t *testing.T) {
	ten := []float64{5, 1, 9, 3, 7, 2, 8, 4, 6, 10}
	m := Summarize("x", "ms", ten)
	if m.P50 != 5 || m.P95 != 10 || m.Max != 10 || m.N != 10 {
		t.Fatalf("ten samples: %+v (p95 of 10 is the maximum)", m)
	}
	twenty := make([]float64, 20)
	for i := range twenty {
		twenty[i] = float64(i + 1)
	}
	if m := Summarize("x", "ms", twenty); m.P50 != 10 || m.P95 != 19 {
		t.Fatalf("twenty samples: %+v", m)
	}
	if m := Summarize("x", "ms", []float64{0.0004, 0.0123}); m.P95 != 0.012 {
		t.Fatalf("sub-millisecond timings keep microsecond resolution: %+v", m)
	}
	if m := Summarize("x", "ms", nil); m.N != 0 || m.P95 != 0 {
		t.Fatalf("empty: %+v", m)
	}
}

func TestSummarizeLeavesInputAlone(t *testing.T) {
	in := []float64{3, 1, 2}
	Summarize("x", "ms", in)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Fatalf("input reordered: %v", in)
	}
}

// TestResultKeyOrder pins the JSON key order of contract:cli.perfbench v1.
func TestResultKeyOrder(t *testing.T) {
	raw, err := Marshal(NewResult("abc", 10, []Metric{Summarize("ci.startup", "ms", []float64{1})}))
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, key := range []string{`"schema"`, `"scenario"`, `"sha"`, `"goos"`, `"goarch"`, `"runs"`, `"injected"`, `"metrics"`,
		`"name"`, `"unit"`, `"p50"`, `"p95"`, `"max"`, `"n"`} {
		at := strings.Index(string(raw), key)
		if at <= last {
			t.Fatalf("key %s out of order in:\n%s", key, raw)
		}
		last = at
	}
	if !strings.HasSuffix(string(raw), "}\n") || !strings.Contains(string(raw), `"schema": "perfbench/v1"`) {
		t.Fatalf("not the indented perfbench/v1 form:\n%s", raw)
	}
}

func TestReadResult(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	raw, _ := Marshal(NewResult("", 1, nil))
	if err := os.WriteFile(good, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadResult(good); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.json")
	doc, _ := json.Marshal(map[string]any{"schema": "other/v9"})
	if err := os.WriteFile(bad, doc, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadResult(bad); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("wrong schema accepted: %v", err)
	}
	if _, err := ReadResult(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestCollectorKeepsFirstAppearanceOrder(t *testing.T) {
	var c Collector
	c.Add("b", "ms", 1)
	c.Add("a", "ms", 2)
	c.Add("b", "ms", 3)
	ms := c.Metrics()
	if len(ms) != 2 || ms[0].Name != "b" || ms[0].N != 2 || ms[1].Name != "a" {
		t.Fatalf("%+v", ms)
	}
}

func TestBudgetFromResult(t *testing.T) {
	base := NewResult("", 10, []Metric{
		Summarize("ci.startup", "ms", []float64{10}),
		Summarize("ci.plan", "ms", []float64{0.04}),
		Summarize("ci.bytes", "B", []float64{500}),
		Summarize("ci.service_start_warm", "ms", []float64{1000}),
	})
	b, err := BudgetFromResult(base, "note", func(n string) bool { return !strings.HasPrefix(n, "ci.service_") })
	if err != nil {
		t.Fatal(err)
	}
	if b.Schema != BudgetSchemaV1 || b.P95MS["ci.startup"] != 13 || b.P95MS["ci.plan"] != 1 {
		t.Fatalf("%+v", b)
	}
	if _, ok := b.P95MS["ci.bytes"]; ok {
		t.Fatal("a non-ms metric got a ms ceiling")
	}
	if _, ok := b.P95MS["ci.service_start_warm"]; ok {
		t.Fatal("the filter was ignored")
	}
	if got := strings.Join(b.Names(), ","); got != "ci.plan,ci.startup" {
		t.Fatalf("names %s", got)
	}
	zero := NewResult("", 1, []Metric{{Name: "ci.x", Unit: "ms"}})
	if _, err := BudgetFromResult(zero, "", nil); err == nil {
		t.Fatal("a zero p95 was given a ceiling")
	}
	if _, err := BudgetFromResult(NewResult("", 1, nil), "", nil); err == nil {
		t.Fatal("an empty baseline produced a budget")
	}
}
