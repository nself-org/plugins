//go:build integration

package sim

// Purpose: the perfbench/v1 document of the node scenario legs (P7-NODE-17): every measured leg adds
// named samples, and TestMain writes one document after the run when -perf-json is set.
// Inputs:  -perf-json <path>; samples from TestScenario2, TestScenario3 and TestWriteLoad.
// Outputs: one JSON file with the command, revision and environment of the run and one entry per
//          metric (count, min, p50, p95, p99, max, mean).
// Constraints: nothing is written when no leg measured anything; the numbers are only ever measured,
//          never defaulted, so a leg that did not run leaves its metric out and `perfbench check`
//          reports it missing.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	perfJSON     = flag.String("perf-json", "", "write the perfbench/v1 measurements of the node scenario legs to this file")
	writeLoadFor = flag.Duration("write-load-for", 10*time.Minute, "how long TestWriteLoad holds the load (the acceptance leg is 10m)")
)

// metric is one perfbench/v1 entry.
type metric struct {
	Name    string  `json:"name"`
	Unit    string  `json:"unit"`
	Leg     string  `json:"leg"`
	Count   int     `json:"count"`
	Min     float64 `json:"min"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
	P99     float64 `json:"p99"`
	Max     float64 `json:"max"`
	Mean    float64 `json:"mean"`
	Context string  `json:"context,omitempty"`
}

var perf struct {
	mu      sync.Mutex
	metrics []metric
}

// recordPerf adds the samples of one metric; an empty sample set records nothing.
func recordPerf(leg, name, unit, context string, samples []float64) {
	if len(samples) == 0 {
		return
	}
	s := append([]float64(nil), samples...)
	sort.Float64s(s)
	sum := 0.0
	for _, v := range s {
		sum += v
	}
	m := metric{Name: name, Unit: unit, Leg: leg, Count: len(s), Min: s[0], P50: quantile(s, 0.50), P95: quantile(s, 0.95),
		P99: quantile(s, 0.99), Max: s[len(s)-1], Mean: sum / float64(len(s)), Context: context}
	perf.mu.Lock()
	perf.metrics = append(perf.metrics, m)
	perf.mu.Unlock()
}

// quantile is the nearest-rank quantile of sorted samples.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q*float64(len(sorted))+0.999999) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// environment names where the numbers were measured.
func environment() map[string]any {
	env := map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH, "cpus": runtime.NumCPU(), "go": runtime.Version()}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		env["kernel"] = strings.TrimSpace(string(b))
	}
	for k, v := range map[string]string{"runner_os": "RUNNER_OS", "runner_image": "ImageOS", "runner_image_version": "ImageVersion",
		"github_run_id": "GITHUB_RUN_ID", "github_run_attempt": "GITHUB_RUN_ATTEMPT", "github_repository": "GITHUB_REPOSITORY"} {
		if s := os.Getenv(v); s != "" {
			env[k] = s
		}
	}
	env["hosted"] = os.Getenv("GITHUB_ACTIONS") == "true"
	return env
}

// revision is the commit of the checkout the legs ran from.
func revision() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func writePerf(path string) error {
	perf.mu.Lock()
	defer perf.mu.Unlock()
	if len(perf.metrics) == 0 {
		return nil
	}
	sort.SliceStable(perf.metrics, func(i, j int) bool { return perf.metrics[i].Name < perf.metrics[j].Name })
	doc := map[string]any{"schema": "perfbench/v1", "suite": "plugins/free/ci/test/sim", "command": strings.Join(os.Args, " "),
		"revision": revision(), "measured_at": time.Now().UTC().Format(time.RFC3339), "environment": environment(), "metrics": perf.metrics}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func TestMain(m *testing.M) {
	flag.Parse()
	code := m.Run()
	if *perfJSON != "" {
		if err := writePerf(*perfJSON); err != nil {
			fmt.Fprintf(os.Stderr, "perf-json: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

// integration fails the leg unless INTEGRATION=1, the opt-in every Docker and load leg of the plugins
// repo uses (simharness skips on its own; the load leg has no Docker, so it asks here).
func integration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run the node scenario legs")
	}
}
