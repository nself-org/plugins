package main

// Purpose: `nself ci runs list [--limit N] [--json]` and `nself ci runs prune
// [--dry-run] [--keep N] [--max-age-days N] [--max-log-bytes N] [--json]`
// (P7-CI-39, Epic D12/X18: history folds into `ci runs`). The queries and the
// retention are internal/history; this file parses flags, opens the store and prints.
// Inputs:  argv after the sub name; env NSELF_CI_HOME, NSELF_CI_REPO;
// <state>/retention.json for the limits.
// Outputs: a table or one v1 envelope; exit 0 ok, 1 usage, 2 infrastructure.
// Constraints: V15 only; read-only commands never create a store; prune never
// removes a running run; flag limits override the retention file for that call.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/history"
)

func init() {
	registerRunsSub("list", runRunsList)
	registerRunsSub("prune", runRunsPrune)
}

type historyFlags struct {
	json, dryRun            bool
	limit, keep, maxAgeDays int
	maxLogBytes             int64
	repo                    string
}

func newHistoryFlagSet(f *historyFlags, prune bool) *flag.FlagSet {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	if prune {
		fs.BoolVar(&f.dryRun, "dry-run", false, "show what would be removed")
		fs.IntVar(&f.keep, "keep", 0, "keep the newest N runs (default from retention.json, else 200)")
		fs.IntVar(&f.maxAgeDays, "max-age-days", 0, "remove runs older than N days (default 30)")
		fs.Int64Var(&f.maxLogBytes, "max-log-bytes", 0, "cap the stored logs at N bytes (default 2 GiB)")
		return fs
	}
	fs.IntVar(&f.limit, "limit", 20, "show at most N runs (newest first)")
	fs.StringVar(&f.repo, "repo", "", "repository root freshness is judged against (env NSELF_CI_REPO; default: current directory)")
	return fs
}

func runRunsList(args []string) int {
	var f historyFlags
	pos, err := parseMixed(newHistoryFlagSet(&f, false), args)
	if err != nil || len(pos) > 0 || f.limit < 1 {
		return usageErr("nself ci runs list [--limit N] [--json] [--repo dir]")
	}
	root, err := evidenceRoot(f.repo)
	if err != nil {
		return exitForError(err)
	}
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	entries, err := history.List(context.Background(), history.Env{Store: st, Root: root, Home: os.Getenv("NSELF_CI_HOME")}, f.limit)
	if err != nil {
		return exitForError(err)
	}
	if f.json {
		if entries == nil {
			entries = []history.Entry{}
		}
		if err = output.WriteData(os.Stdout, "ci runs list", entries, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if len(entries) == 0 {
		fmt.Println("no runs recorded yet")
		return 0
	}
	fmt.Printf("%-34s %-8s %-9s %-8s %-8s %s\n", "RUN", "RESULT", "DURATION", "REVISION", "BINDING", "FRESHNESS")
	for _, e := range entries {
		fmt.Printf("%-34s %-8s %-9s %-8.7s %-8s %s\n", e.ID, e.Result, durationText(e.DurationMS), firstOf(e.Revision, "-"), e.Binding, e.Freshness)
	}
	return 0
}

func durationText(ms *int64) string {
	switch {
	case ms == nil:
		return "-"
	case *ms < 1000:
		return strconv.FormatInt(*ms, 10) + "ms"
	case *ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(*ms)/1000)
	}
	return fmt.Sprintf("%dm%02ds", *ms/60_000, (*ms/1000)%60)
}

func runRunsPrune(args []string) int {
	var f historyFlags
	pos, err := parseMixed(newHistoryFlagSet(&f, true), args)
	if err != nil || len(pos) > 0 || f.keep < 0 || f.maxAgeDays < 0 || f.maxLogBytes < 0 {
		return usageErr("nself ci runs prune [--dry-run] [--keep N] [--max-age-days N] [--max-log-bytes N] [--json]")
	}
	home := os.Getenv("NSELF_CI_HOME")
	pol, perr := history.LoadPolicy(home)
	if perr != nil {
		fmt.Fprintf(os.Stderr, "note: %s is unusable (%v); using the defaults\n", history.ConfigPath(home), perr)
	}
	pol.KeepRuns, pol.MaxAgeDays = nonZero(f.keep, pol.KeepRuns), nonZero(f.maxAgeDays, pol.MaxAgeDays)
	if f.maxLogBytes > 0 {
		pol.MaxLogBytes = f.maxLogBytes
	}
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	if st == nil {
		if f.json {
			_ = output.WriteData(os.Stdout, "ci runs prune", history.PruneResult{Runs: []string{}, Orphans: []string{}, DryRun: f.dryRun}, nil)
			return 0
		}
		fmt.Println("no run history on this machine; nothing to prune")
		return 0
	}
	defer func() { _ = st.Close() }()
	res, err := history.Prune(context.Background(), st, history.PruneParams{Home: home, Policy: pol, DryRun: f.dryRun})
	if err != nil {
		return logsFail(logsFlags{json: f.json}, "ci runs prune", err)
	}
	if f.json {
		if err = output.WriteData(os.Stdout, "ci runs prune", res, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	verb := "removed"
	if res.DryRun {
		verb = "would remove"
	}
	fmt.Printf("%s %d run(s) and %d orphan log dir(s), %d bytes of stored logs\n", verb, len(res.Runs), len(res.Orphans), res.FreedBytes)
	return 0
}

func nonZero(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
