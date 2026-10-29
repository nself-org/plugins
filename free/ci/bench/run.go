package bench

// Purpose: the harness loop. Each iteration times the startup of the binary,
// the in-process scenarios, and the end-to-end gate on the tiny and medium
// fixtures, v1.4 and V15 interleaved (the order alternates per iteration so
// neither side always runs on a warm cache the other filled).
// Inputs:  the nself-ci binary, run and warm-up counts, a scratch directory.
// Outputs: a perfbench/v1 Result.
// Constraints: warm-up iterations run the same code and are discarded; every
// end-to-end run must exit 0 (a failing gate is a fixture bug, not a timing);
// the same binary serves both modes, selected by NSELF_V15 as in production
// (compat.V15), and both use one NSELF_CI_HOME so the store is warm for both.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Metric names (Ticket constraint; unit ms).
const (
	MetricStartup       = "ci.startup"
	MetricDetect        = "ci.detect"
	MetricPlan          = "ci.plan"
	MetricStoreOpen     = "ci.store_open"
	MetricExecOverhead  = "ci.exec_overhead"
	MetricExecBaseline  = "ci.exec_baseline"
	MetricCapacityProbe = "ci.capacity_probe"
	MetricMediumV14     = "ci.e2e_medium.v14"
	MetricMediumV15     = "ci.e2e_medium.v15"
	MetricServiceWarm   = "ci.service_start_warm"
	MetricServiceCold   = "ci.service_start_cold"
)

// Config is one harness run.
type Config struct {
	Bin        string // the nself-ci binary
	Runs       int    // measured iterations
	Warmup     int    // discarded iterations
	Dir        string // scratch directory, created by the caller
	SHA        string // revision measured, "" when unknown
	SkipMedium bool
	Services   *ServiceConfig // nil skips the service scenarios
	Log        io.Writer
}

func (c Config) logf(format string, a ...any) {
	if c.Log != nil {
		_, _ = fmt.Fprintf(c.Log, format+"\n", a...)
	}
}

// mode is one engine under test.
type mode struct {
	label string
	env   string // value of NSELF_V15
	args  func(dir string) []string
}

var modes = []mode{
	{"v14", "0", func(dir string) []string { return []string{"--no-status", "--no-gitleaks", dir} }},
	{"v15", "1", func(dir string) []string { return []string{"--no-status", "--pipeline", "gate", dir} }},
}

// Run executes the harness.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Runs < 1 || cfg.Warmup < 0 {
		return Result{}, fmt.Errorf("runs must be >= 1 and warmup >= 0")
	}
	repos := map[string]string{Tiny: filepath.Join(cfg.Dir, Tiny)}
	if !cfg.SkipMedium {
		repos[Medium] = filepath.Join(cfg.Dir, Medium)
	}
	for name, dir := range repos {
		if err := Materialize(ctx, name, dir); err != nil {
			return Result{}, err
		}
	}
	home := filepath.Join(cfg.Dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return Result{}, err
	}
	inproc, err := PrepareInproc(repos[Tiny], filepath.Join(cfg.Dir, "scratch"))
	if err != nil {
		return Result{}, err
	}
	var col Collector
	for i := -cfg.Warmup; i < cfg.Runs; i++ {
		keep := i >= 0
		add := func(name string, ms float64) {
			if keep {
				col.Add(name, "ms", ms)
			}
		}
		if err := iteration(ctx, cfg, repos, home, inproc, i, add); err != nil {
			return Result{}, err
		}
		cfg.logf("iteration %d/%d done", i+cfg.Warmup+1, cfg.Runs+cfg.Warmup)
	}
	if cfg.Services != nil {
		if err := measureServices(ctx, cfg, home, &col); err != nil {
			return Result{}, err
		}
	}
	return NewResult(cfg.SHA, cfg.Runs, col.Metrics()), nil
}

// iteration runs every scenario once.
func iteration(ctx context.Context, cfg Config, repos map[string]string, home string, p *Inproc, i int, add func(string, float64)) error {
	env := func(v15 string) []string {
		return append(baseEnv(), "NSELF_CI_HOME="+home, "NSELF_TELEMETRY_OPT_OUT=1", "NSELF_V15="+v15)
	}
	r, err := must(ctx, []string{cfg.Bin, "--help"}, cfg.Dir, env("1"))
	if err != nil {
		return err
	}
	add(MetricStartup, r.MS())
	steps := []struct {
		name string
		fn   func() error
	}{
		{MetricDetect, p.Detect}, {MetricPlan, p.Plan}, {MetricStoreOpen, p.StoreOpen},
	}
	for _, s := range steps {
		wall, err := timeIt(s.fn)
		if err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		add(s.name, wall)
	}
	if r, err = p.ExecOverhead(ctx); err != nil {
		return err
	}
	add(MetricExecOverhead, r.MS())
	if r, err = p.RawSpawn(ctx); err != nil {
		return err
	}
	add(MetricExecBaseline, r.MS())
	wall, _ := timeIt(func() error { CapacityProbe(); return nil })
	add(MetricCapacityProbe, wall)
	order := modes
	if i%2 != 0 {
		order = []mode{modes[1], modes[0]}
	}
	for _, name := range []string{Tiny, Medium} {
		dir, ok := repos[name]
		if !ok {
			continue
		}
		for _, m := range order {
			r, err := must(ctx, append([]string{cfg.Bin}, m.args(dir)...), dir, env(m.env))
			if err != nil {
				return fmt.Errorf("e2e %s %s: %w", name, m.label, err)
			}
			add(fmt.Sprintf("ci.e2e_%s.%s", name, m.label), r.MS())
		}
	}
	return nil
}
