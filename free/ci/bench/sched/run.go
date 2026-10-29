package sched

// Purpose: the schedbench loop. Each iteration makes one scheduling decision and
// runs the V15 gate once on the tiny fixture, then pairs them: the scheduling
// share of that run is (decisions the run makes x the measured decision cost) /
// the run's own wall time.
// Inputs:  the nself-ci binary, run and warm-up counts, a scratch directory.
// Outputs: a perfbench/v1 Result with sched.place_ms, dispatch.admit_ms (ms) and
// sched.share_tiny_pct (pct).
// Constraints: warm-up iterations run the same code and are discarded. The engine
// does not time its own scheduling compute (schedtel wait_ms includes waiting for
// a slot), so the numerator is the decision cost measured here in the same
// iteration and the tiny pipeline's planned job count, one decision per job. That
// is an upper bound: the dispatcher builds the world once per job in this model.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/bench"
)

// Metric names (unit ms except the share).
const (
	MetricPlace = "sched.place_ms"
	MetricAdmit = "dispatch.admit_ms"
	MetricShare = "sched.share_tiny_pct"
	// ScenarioName is the scenario field of the result document.
	ScenarioName = "ci-sched-overhead"
)

// Config is one harness run.
type Config struct {
	Bin    string
	Runs   int
	Warmup int
	Dir    string // scratch directory, created by the caller
	SHA    string
	Log    io.Writer
}

// Run executes the harness.
func Run(ctx context.Context, cfg Config) (bench.Result, error) {
	if cfg.Runs < 1 || cfg.Warmup < 0 {
		return bench.Result{}, fmt.Errorf("runs must be >= 1 and warmup >= 0")
	}
	repo := filepath.Join(cfg.Dir, bench.Tiny)
	if err := bench.Materialize(ctx, bench.Tiny, repo); err != nil {
		return bench.Result{}, err
	}
	home := filepath.Join(cfg.Dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return bench.Result{}, err
	}
	inproc, err := bench.PrepareInproc(repo, filepath.Join(cfg.Dir, "scratch"))
	if err != nil {
		return bench.Result{}, err
	}
	jobs, err := inproc.PlannedJobs()
	if err != nil || jobs < 1 {
		return bench.Result{}, fmt.Errorf("tiny pipeline job count %d: %v", jobs, err)
	}
	var col bench.Collector
	for i := -cfg.Warmup; i < cfg.Runs; i++ {
		d, err := Decide(ctx, filepath.Join(cfg.Dir, fmt.Sprintf("decide-%d", i+cfg.Warmup)))
		if err != nil {
			return bench.Result{}, fmt.Errorf("decision: %w", err)
		}
		e2e, err := bench.GateV15(ctx, cfg.Bin, repo, home)
		if err != nil {
			return bench.Result{}, err
		}
		if i >= 0 {
			col.Add(MetricPlace, "ms", d.PlaceMS)
			col.Add(MetricAdmit, "ms", d.AdmitMS)
			col.Add(MetricShare, "pct", 100*float64(jobs)*d.AdmitMS/e2e.MS())
		}
		if cfg.Log != nil {
			_, _ = fmt.Fprintf(cfg.Log, "iteration %d/%d: place %.3f ms, admit %.3f ms, e2e %.0f ms, %d jobs\n", i+cfg.Warmup+1, cfg.Runs+cfg.Warmup, d.PlaceMS, d.AdmitMS, e2e.MS(), jobs)
		}
	}
	res := bench.NewResult(cfg.SHA, cfg.Runs, col.Metrics())
	res.Scenario = ScenarioName
	return res, nil
}
