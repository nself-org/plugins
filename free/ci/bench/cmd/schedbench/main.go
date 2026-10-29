// Command schedbench measures the scheduling cost of the V15 ci engine
// (P7-SCHED-14) and emits contract:cli.perfbench v1 result JSON.
//
//	schedbench [-runs 20] [-warmup 2] [-bin path] [-out file] [-json]
//	schedbench -write-budget sched-budget.json -share-budget share.json -in run1.json,run2.json
//	schedbench -share-check share.json -in result.json
//
// The millisecond metrics are checked by the cli's `perfbench check`; the share
// (unit pct) is checked by -share-check. Run it from the free/ci module root.
// Exit 0 on success, 1 when a check fails, 2 on a usage or environment error.
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

	"github.com/nself-org/plugins/free/ci/bench"
	schedbench "github.com/nself-org/plugins/free/ci/bench/sched"
)

func main() { os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr)) }

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }

type options struct {
	runs, warmup                    int
	bin, sha, in, out, note         string
	writeBudget, shareBudget, check string
	jsonOut                         bool
}

func realMain(args []string, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet("schedbench", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&o.runs, "runs", 20, "measured iterations")
	fs.IntVar(&o.warmup, "warmup", 2, "discarded warm-up iterations")
	fs.StringVar(&o.bin, "bin", "", "nself-ci binary (default: build ./cmd)")
	fs.StringVar(&o.sha, "sha", "", "revision measured (default: git HEAD of the working directory)")
	fs.StringVar(&o.in, "in", "", "result file (-share-check); with -write-budget a comma list of baseline runs")
	fs.StringVar(&o.out, "out", "", "write the result JSON here")
	fs.StringVar(&o.note, "note", "", "_note text of a derived budget")
	fs.StringVar(&o.writeBudget, "write-budget", "", "derive the perfbench budget (ms metrics) from -in and write it here")
	fs.StringVar(&o.shareBudget, "share-budget", "", "with -write-budget: also write the share ceiling here")
	fs.StringVar(&o.check, "share-check", "", "check -in against this share ceiling file")
	fs.BoolVar(&o.jsonOut, "json", false, "print the result JSON to stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch {
	case o.check != "":
		return checkCmd(o, stdout, stderr)
	case o.writeBudget != "":
		return budgetCmd(o, stderr)
	}
	return runCmd(o, stdout, stderr)
}

func readAll(list string) ([]bench.Result, error) {
	var runs []bench.Result
	for _, p := range strings.Split(list, ",") {
		r, err := bench.ReadResult(p)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func checkCmd(o options, stdout, stderr io.Writer) int {
	b, err := schedbench.LoadShare(o.check)
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	res, err := bench.ReadResult(o.in)
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	if v := schedbench.CheckShare(b, res); v != "" {
		say(stderr, "schedbench: FAIL\n%s", v)
		return 1
	}
	m, _ := res.Find(schedbench.MetricShare)
	say(stdout, "%s: p95 %.2f%% within the ceiling %.1f%%", schedbench.MetricShare, m.P95, b.P95Pct)
	return 0
}

func budgetCmd(o options, stderr io.Writer) int {
	runs, err := readAll(o.in)
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	worst, err := bench.WorstOf(runs)
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	b, err := bench.BudgetFromResult(worst, o.note, func(name string) bool { return name != schedbench.MetricShare })
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	if err = writeJSON(o.writeBudget, b); err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	if o.shareBudget != "" {
		s, err := schedbench.ShareFromBaseline(runs, o.note)
		if err == nil {
			err = writeJSON(o.shareBudget, s)
		}
		if err != nil {
			say(stderr, "schedbench: %v", err)
			return 2
		}
	}
	return 0
}

func writeJSON(path string, v any) error {
	raw, err := bench.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func runCmd(o options, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "schedbench-")
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if o.bin == "" {
		if o.bin, err = bench.BuildBinary(ctx, ".", filepath.Join(dir, "nself-ci")); err != nil {
			say(stderr, "schedbench: %v", err)
			return 2
		}
	}
	if o.sha == "" {
		o.sha = bench.TreeSHA(ctx, ".")
	}
	res, err := schedbench.Run(ctx, schedbench.Config{Bin: o.bin, Runs: o.runs, Warmup: o.warmup, Dir: dir, SHA: o.sha, Log: stderr})
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	raw, err := bench.Marshal(res)
	if err == nil && o.out != "" {
		err = os.WriteFile(o.out, raw, 0o644)
	}
	if err != nil {
		say(stderr, "schedbench: %v", err)
		return 2
	}
	if o.jsonOut {
		_, _ = stdout.Write(raw)
	}
	say(stderr, "%s", bench.Format(res))
	return 0
}
