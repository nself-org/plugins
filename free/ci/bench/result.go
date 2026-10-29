// Package bench measures the local overhead of the V15 ci engine against the
// frozen v1.4 gate (P7-CI-46).
//
// Purpose: produce contract:cli.perfbench v1 result JSON that the cli's own
// `perfbench check` accepts, set the local fast-path budget from a recorded
// baseline, and assert the V15/v1.4 wall-time ratio.
// Inputs:  the nself-ci binary under test, fixture repos (testdata), the host.
// Outputs: Result documents (perfbench/v1), Budget documents
// (perfbench-budget/v1) and the ratio verdict.
// Constraints: the JSON field order and the nearest-rank percentile match
// cli:tools/perfbench exactly, because `perfbench check` is the one gate that
// reads these files; scenarios stay in this module (perfbench scenarios are cli
// code and the plugin cannot register one).
package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
)

// SchemaV1 is the value of the "schema" field of every result document.
const SchemaV1 = "perfbench/v1"

// Metric is one summarized measurement. Field order is the JSON key order.
type Metric struct {
	Name string  `json:"name"`
	Unit string  `json:"unit"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	Max  float64 `json:"max"`
	N    int     `json:"n"`
}

// Result is the perfbench/v1 document. Field order is the JSON key order.
type Result struct {
	Schema   string   `json:"schema"`
	Scenario string   `json:"scenario"`
	SHA      string   `json:"sha"`
	GOOS     string   `json:"goos"`
	GOARCH   string   `json:"goarch"`
	Runs     int      `json:"runs"`
	Injected bool     `json:"injected"`
	Metrics  []Metric `json:"metrics"`
}

// Scenario is the scenario name of every result this package emits.
const Scenario = "ci-local-overhead"

// NewResult fills the environment fields of a result. sha names what was
// measured (the plugin revision); it may be empty when unknown.
func NewResult(sha string, runs int, metrics []Metric) Result {
	return Result{Schema: SchemaV1, Scenario: Scenario, SHA: sha, GOOS: runtime.GOOS,
		GOARCH: runtime.GOARCH, Runs: runs, Metrics: metrics}
}

// Find returns the metric called name.
func (r Result) Find(name string) (Metric, bool) {
	for _, m := range r.Metrics {
		if m.Name == name {
			return m, true
		}
	}
	return Metric{}, false
}

// Marshal renders v as 2-space-indented JSON with a trailing newline.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadResult loads and validates a perfbench/v1 document.
func ReadResult(path string) (Result, error) {
	var r Result
	raw, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("result %s: %w", path, err)
	}
	if r.Schema != SchemaV1 {
		return r, fmt.Errorf("result %s: schema %q, want %q", path, r.Schema, SchemaV1)
	}
	return r, nil
}

// percentile returns the nearest-rank p-th percentile (0 < p <= 100) of
// samples; the slice is not modified and an empty slice yields 0.
func percentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	s := append([]float64(nil), samples...)
	sort.Float64s(s)
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	rank = min(max(rank, 1), len(s))
	return s[rank-1]
}

// round3 rounds to a microsecond. perfbench itself rounds to 0.1 ms; the planner
// takes tens of microseconds, and a p95 rounded to 0.0 is not a usable timing.
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// Summarize turns the raw samples of one metric into the JSON metric:
// nearest-rank p50, p95 and max, each rounded to 0.001 ms.
func Summarize(name, unit string, samples []float64) Metric {
	top := 0.0
	for i, v := range samples {
		if i == 0 || v > top {
			top = v
		}
	}
	return Metric{Name: name, Unit: unit, P50: round3(percentile(samples, 50)),
		P95: round3(percentile(samples, 95)), Max: round3(top), N: len(samples)}
}

// Collector gathers samples per metric and keeps first-appearance order.
type Collector struct {
	order []string
	unit  map[string]string
	vals  map[string][]float64
}

// Add records one sample (milliseconds unless unit says otherwise).
func (c *Collector) Add(name, unit string, v float64) {
	if c.unit == nil {
		c.unit, c.vals = map[string]string{}, map[string][]float64{}
	}
	if _, seen := c.unit[name]; !seen {
		c.order = append(c.order, name)
		c.unit[name] = unit
	}
	c.vals[name] = append(c.vals[name], v)
}

// Metrics summarizes every collected metric in first-appearance order.
func (c *Collector) Metrics() []Metric {
	out := make([]Metric, 0, len(c.order))
	for _, name := range c.order {
		out = append(out, Summarize(name, c.unit[name], c.vals[name]))
	}
	return out
}
