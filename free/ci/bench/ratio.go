package bench

// Purpose: the relative budget of the local fast path (Ticket constraint):
// ci.e2e_tiny.v15 median <= ci.e2e_tiny.v14 median * 1.10 + 250 ms.
// Inputs:  a Result carrying both e2e metrics, the recorded exceptions, a clock.
// Outputs: one line per violated or passed assertion.
// Constraints: a miss fails unless the owner accepted a written exception for
// this platform (cause, gap, expiry) and it has not expired; an exception
// without accepted_by is a request, not an acceptance, and does not count. The
// medium fixture is recorded but not asserted (the Ticket budgets tiny only).

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Relative budget terms, from the Ticket.
const (
	RatioFactor  = 1.10
	RatioSlackMS = 250.0
)

// Metric names of the end-to-end comparison.
const (
	MetricTinyV14 = "ci.e2e_tiny.v14"
	MetricTinyV15 = "ci.e2e_tiny.v15"
)

// ExceptionsSchemaV1 is the value of the "schema" field of the exceptions file.
const ExceptionsSchemaV1 = "cibench-ratio-exceptions/v1"

// Exception is one owner-accepted miss of the relative budget.
type Exception struct {
	Platform   string  `json:"platform"` // goos/goarch
	Cause      string  `json:"cause"`
	GapMS      float64 `json:"gap_ms"`  // the largest accepted excess over the budget
	Expires    string  `json:"expires"` // YYYY-MM-DD, inclusive
	AcceptedBy string  `json:"accepted_by"`
	AcceptedOn string  `json:"accepted_on"`
}

// Exceptions is the exceptions file.
type Exceptions struct {
	Schema     string      `json:"schema"`
	Exceptions []Exception `json:"exceptions"`
}

// LoadExceptions reads the exceptions file; a missing file is no exceptions.
func LoadExceptions(path string) (Exceptions, error) {
	var ex Exceptions
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Exceptions{Schema: ExceptionsSchemaV1}, nil
	}
	if err != nil {
		return ex, err
	}
	if err := json.Unmarshal(raw, &ex); err != nil {
		return ex, fmt.Errorf("exceptions %s: %w", path, err)
	}
	if ex.Schema != ExceptionsSchemaV1 {
		return ex, fmt.Errorf("exceptions %s: schema %q, want %q", path, ex.Schema, ExceptionsSchemaV1)
	}
	return ex, nil
}

// covers reports whether an accepted, unexpired exception admits a miss of gap
// ms on platform at time now.
func (ex Exceptions) covers(platform string, gap float64, now time.Time) (Exception, bool) {
	for _, e := range ex.Exceptions {
		until, err := time.Parse("2006-01-02", e.Expires)
		if e.Platform != platform || e.AcceptedBy == "" || err != nil || gap > e.GapMS {
			continue
		}
		if now.Before(until.Add(24 * time.Hour)) {
			return e, true
		}
	}
	return Exception{}, false
}

// RatioLimit is the V15 median the relative budget allows for a v1.4 median.
func RatioLimit(v14 float64) float64 { return v14*RatioFactor + RatioSlackMS }

// RatioCheck asserts the relative budget on res. It fails closed: a missing
// metric is a violation.
func RatioCheck(res Result, ex Exceptions, now time.Time) (violations, passes []string) {
	old, ok14 := res.Find(MetricTinyV14)
	cur, ok15 := res.Find(MetricTinyV15)
	if !ok14 || !ok15 {
		return []string{MetricTinyV14 + " and " + MetricTinyV15 + " must both be in the result"}, nil
	}
	limit := RatioLimit(old.P50)
	line := fmt.Sprintf("%s median %.1f ms vs limit %.1f ms (%s median %.1f ms x %.2f + %.0f ms)",
		MetricTinyV15, cur.P50, limit, MetricTinyV14, old.P50, RatioFactor, RatioSlackMS)
	if cur.P50 <= limit {
		return nil, []string{line}
	}
	platform := res.GOOS + "/" + res.GOARCH
	if e, ok := ex.covers(platform, cur.P50-limit, now); ok {
		return nil, []string{line + fmt.Sprintf("; MISS accepted by %s on %s until %s: %s", e.AcceptedBy, e.AcceptedOn, e.Expires, e.Cause)}
	}
	return []string{line + fmt.Sprintf("; over by %.1f ms and no accepted exception for %s", cur.P50-limit, platform)}, nil
}
