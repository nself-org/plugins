// Command cibench is the process-level harness of the V15 ci engine
// (P7-CI-46). It times the nself-ci binary against the frozen v1.4 gate on the
// fixture repos and emits contract:cli.perfbench v1 result JSON.
//
//	cibench [-runs 10] [-warmup 2] [-bin path] [-json] [-services [-cold]]
//	cibench -ratio-check -in result.json [-exceptions file]
//	cibench -write-budget perf-budget.json -in run1.json,run2.json,run3.json [-only-core]
//
// Run it from the free/ci module root; without -bin it builds ./cmd first.
// Results go to stdout with -json (everything else goes to stderr). Exit 0 on
// success, 1 when a check fails, 2 on a usage or environment error.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/bench"
)

func main() { os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr)) }

type options struct {
	runs, warmup, coldRuns        int
	bin, nself, sha, in, out      string
	exceptions, writeBudget, note string
	jsonOut, ratio, services      bool
	cold, skipMedium, onlyCore    bool
}

func realMain(args []string, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("cibench", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.runs, "runs", 10, "measured iterations")
	fs.IntVar(&o.warmup, "warmup", 2, "discarded warm-up iterations")
	fs.IntVar(&o.coldRuns, "cold-runs", 0, "cold service samples (default: -runs)")
	fs.StringVar(&o.bin, "bin", "", "nself-ci binary (default: build ./cmd)")
	fs.StringVar(&o.nself, "nself", "nself", "nself CLI for -services")
	fs.StringVar(&o.sha, "sha", "", "revision measured (default: git HEAD of the working directory)")
	fs.StringVar(&o.in, "in", "", "perfbench/v1 result file (-ratio-check); with -write-budget a comma list of baseline runs, worst p95 per metric")
	fs.StringVar(&o.out, "out", "", "write the result JSON here instead of stdout")
	fs.StringVar(&o.exceptions, "exceptions", "bench/ratio-exceptions.json", "owner-accepted ratio exceptions")
	fs.StringVar(&o.writeBudget, "write-budget", "", "derive a budget file from -in and write it here")
	fs.StringVar(&o.note, "note", "", "_note text of a derived budget")
	fs.BoolVar(&o.jsonOut, "json", false, "print the result JSON to stdout")
	fs.BoolVar(&o.ratio, "ratio-check", false, "assert the V15/v1.4 relative budget on -in")
	fs.BoolVar(&o.services, "services", false, "also measure service start (needs Docker and a v1.5 nself CLI)")
	fs.BoolVar(&o.cold, "cold", false, "with -services: also measure the cold start (removes the project's images)")
	fs.BoolVar(&o.skipMedium, "skip-medium", false, "skip the medium fixture (needs node and npm)")
	fs.BoolVar(&o.onlyCore, "only-core", false, "with -write-budget: leave service metrics out")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch {
	case o.ratio:
		return ratioCmd(o, stdout, stderr)
	case o.writeBudget != "":
		return budgetCmd(o, stderr)
	}
	return runCmd(o, stdout, stderr)
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }

// ratioCmd implements -ratio-check.
func ratioCmd(o options, stdout, stderr io.Writer) int {
	if o.in == "" {
		say(stderr, "cibench: -ratio-check needs -in")
		return 2
	}
	res, err := bench.ReadResult(o.in)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	ex, err := bench.LoadExceptions(o.exceptions)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	violations, passes := bench.RatioCheck(res, ex, time.Now())
	for _, p := range passes {
		say(stdout, "%s", p)
	}
	if len(violations) > 0 {
		say(stderr, "cibench: FAIL\n%s", strings.Join(violations, "\n"))
		return 1
	}
	return 0
}

// budgetCmd implements -write-budget.
func budgetCmd(o options, stderr io.Writer) int {
	if o.in == "" {
		say(stderr, "cibench: -write-budget needs -in")
		return 2
	}
	var runs []bench.Result
	for _, path := range strings.Split(o.in, ",") {
		r, err := bench.ReadResult(path)
		if err != nil {
			say(stderr, "cibench: %v", err)
			return 2
		}
		runs = append(runs, r)
	}
	res, err := bench.WorstOf(runs)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	keep := func(name string) bool {
		// The v1.4 gate is frozen: its absolute time is the yardstick of the
		// relative budget, not something to guard (a noisy runner would fail a
		// budget on code nobody changes).
		if informational[name] || strings.HasSuffix(name, ".v14") {
			return false
		}
		return !o.onlyCore || !strings.HasPrefix(name, "ci.service_")
	}
	b, err := bench.BudgetFromResult(res, o.note, keep)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	raw, err := bench.Marshal(b)
	if err == nil {
		err = os.WriteFile(o.writeBudget, raw, 0o644)
	}
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	return 0
}

// informational metrics are recorded to read the others against; they get no ceiling.
var informational = map[string]bool{bench.MetricExecBaseline: true, bench.MetricCapacityProbe: true}

// runCmd measures and prints the result.
func runCmd(o options, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "cibench-")
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if o.bin == "" {
		if o.bin, err = bench.BuildBinary(ctx, ".", filepath.Join(dir, "nself-ci")); err != nil {
			say(stderr, "cibench: %v", err)
			return 2
		}
	}
	if o.sha == "" {
		o.sha = bench.TreeSHA(ctx, ".")
	}
	cfg := bench.Config{Bin: o.bin, Runs: o.runs, Warmup: o.warmup, Dir: dir, SHA: o.sha,
		SkipMedium: o.skipMedium, Log: stderr}
	if o.services {
		cfg.Services = &bench.ServiceConfig{Nself: o.nself, Cold: o.cold, ColdRuns: o.coldRuns}
	}
	res, err := bench.Run(ctx, cfg)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	raw, err := bench.Marshal(res)
	if err != nil {
		say(stderr, "cibench: %v", err)
		return 2
	}
	if o.out != "" {
		if err := os.WriteFile(o.out, raw, 0o644); err != nil {
			say(stderr, "cibench: %v", err)
			return 2
		}
	}
	if o.jsonOut {
		_, _ = stdout.Write(raw)
	}
	say(stderr, "%s", bench.Format(res))
	return 0
}
