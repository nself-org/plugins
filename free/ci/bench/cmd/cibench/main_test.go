package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/bench"
)

func writeResult(t *testing.T, dir string, v14, v15 float64) string {
	t.Helper()
	r := bench.NewResult("abc", 10, []bench.Metric{
		{Name: bench.MetricTinyV14, Unit: "ms", P50: v14, P95: v14, Max: v14, N: 10},
		{Name: bench.MetricTinyV15, Unit: "ms", P50: v15, P95: v15, Max: v15, N: 10},
		{Name: bench.MetricExecBaseline, Unit: "ms", P50: 2, P95: 3, Max: 3, N: 10},
		{Name: bench.MetricServiceWarm, Unit: "ms", P50: 900, P95: 1000, Max: 1000, N: 10},
	})
	raw, err := bench.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRatioCheckExitCodes(t *testing.T) {
	dir := t.TempDir()
	ex := filepath.Join(dir, "none.json")
	for _, c := range []struct {
		name    string
		v14     float64
		v15     float64
		args    []string
		want    int
		wantOut string
	}{
		{"pass", 300, 500, nil, 0, "median 500.0 ms"},
		{"fail", 300, 5000, nil, 1, ""},
		{"no input", 0, 0, []string{"-ratio-check"}, 2, ""},
	} {
		in := writeResult(t, dir, c.v14, c.v15)
		args := c.args
		if args == nil {
			args = []string{"-ratio-check", "-in", in, "-exceptions", ex}
		}
		var out, errw bytes.Buffer
		if got := realMain(args, &out, &errw); got != c.want || !strings.Contains(out.String(), c.wantOut) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", c.name, got, out.String(), errw.String())
		}
	}
}

func TestWriteBudget(t *testing.T) {
	dir := t.TempDir()
	in := writeResult(t, dir, 300, 500)
	out := filepath.Join(dir, "budget.json")
	var so, se bytes.Buffer
	if got := realMain([]string{"-write-budget", out, "-in", in, "-only-core", "-note", "n"}, &so, &se); got != 0 {
		t.Fatalf("exit %d: %s", got, se.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"perfbench-budget/v1"`) || !strings.Contains(s, `"ci.e2e_tiny.v15": 625`) {
		t.Fatalf("budget:\n%s", s)
	}
	if strings.Contains(s, "service_start") || strings.Contains(s, "exec_baseline") || strings.Contains(s, ".v14") {
		t.Fatalf("service, informational and v1.4 metrics must get no ceiling:\n%s", s)
	}
	if got := realMain([]string{"-write-budget", out}, &so, &se); got != 2 {
		t.Fatalf("missing -in must be a usage error, got %d", got)
	}
}

func TestBadFlag(t *testing.T) {
	var so, se bytes.Buffer
	if got := realMain([]string{"-nope"}, &so, &se); got != 2 {
		t.Fatalf("exit %d", got)
	}
}
