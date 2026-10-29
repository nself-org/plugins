package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tinyResult(v14, v15 float64) Result {
	r := NewResult("", 10, []Metric{
		{Name: MetricTinyV14, Unit: "ms", P50: v14, P95: v14, Max: v14, N: 10},
		{Name: MetricTinyV15, Unit: "ms", P50: v15, P95: v15, Max: v15, N: 10},
	})
	r.GOOS, r.GOARCH = "linux", "amd64"
	return r
}

func TestRatioLimit(t *testing.T) {
	if got := RatioLimit(1000); got != 1350 {
		t.Fatalf("limit %v, want 1000 x 1.10 + 250", got)
	}
}

func TestRatioCheck(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	accepted := Exception{Platform: "linux/amd64", Cause: "ps polling", GapMS: 800, Expires: "2026-11-18", AcceptedBy: "owner", AcceptedOn: "2026-10-18"}
	cases := []struct {
		name     string
		res      Result
		ex       []Exception
		wantFail bool
		wantText string
	}{
		{"within budget", tinyResult(300, 580), nil, false, "median 580.0 ms vs limit 580.0 ms"},
		{"over budget", tinyResult(300, 581), nil, true, "no accepted exception for linux/amd64"},
		{"accepted exception", tinyResult(300, 1100), []Exception{accepted}, false, "MISS accepted by owner"},
		{"request without acceptance", tinyResult(300, 1100), []Exception{{Platform: "linux/amd64", GapMS: 800, Expires: "2026-11-18"}}, true, "no accepted exception"},
		{"expired", tinyResult(300, 1100), []Exception{{Platform: "linux/amd64", GapMS: 800, Expires: "2026-10-17", AcceptedBy: "owner"}}, true, "no accepted exception"},
		{"last day counts", tinyResult(300, 1100), []Exception{{Platform: "linux/amd64", GapMS: 800, Expires: "2026-10-18", AcceptedBy: "owner"}}, false, "MISS accepted"},
		{"other platform", tinyResult(300, 1100), []Exception{{Platform: "darwin/arm64", GapMS: 800, Expires: "2026-11-18", AcceptedBy: "owner"}}, true, "no accepted exception"},
		{"gap larger than accepted", tinyResult(300, 2000), []Exception{accepted}, true, "no accepted exception"},
		{"missing metric", NewResult("", 1, nil), nil, true, "must both be in the result"},
	}
	for _, c := range cases {
		violations, passes := RatioCheck(c.res, Exceptions{Exceptions: c.ex}, now)
		out := strings.Join(append(violations, passes...), "\n")
		if (len(violations) > 0) != c.wantFail || !strings.Contains(out, c.wantText) {
			t.Errorf("%s: fail=%v out=%q", c.name, len(violations) > 0, out)
		}
	}
}

func TestLoadExceptions(t *testing.T) {
	dir := t.TempDir()
	if ex, err := LoadExceptions(filepath.Join(dir, "none.json")); err != nil || len(ex.Exceptions) != 0 {
		t.Fatalf("a missing file is no exceptions: %+v %v", ex, err)
	}
	path := filepath.Join(dir, "ex.json")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"schema":"cibench-ratio-exceptions/v1","exceptions":[{"platform":"linux/amd64","gap_ms":5,"expires":"2026-12-01","accepted_by":"owner"}]}`)
	if ex, err := LoadExceptions(path); err != nil || len(ex.Exceptions) != 1 {
		t.Fatalf("%+v %v", ex, err)
	}
	write(`{"schema":"x","exceptions":[]}`)
	if _, err := LoadExceptions(path); err == nil {
		t.Fatal("wrong schema accepted")
	}
	write(`{`)
	if _, err := LoadExceptions(path); err == nil {
		t.Fatal("broken JSON accepted")
	}
}

// TestShippedExceptionsFile keeps the file the workflow reads valid.
func TestShippedExceptionsFile(t *testing.T) {
	if _, err := LoadExceptions("ratio-exceptions.json"); err != nil {
		t.Fatal(err)
	}
}
