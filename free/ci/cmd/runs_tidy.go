package main

// Purpose: `nself ci runs prune --tidy [--apply] [--json]` (P7-AUTO-20). It composes the run
// history retention prune (CI-39), the cache prune and the orphan job-root sweep (TRUST-09)
// with the operator's free-space target (tidy.free_target_mb) and cache cap (tidy.cache_cap_mb).
// Inputs:  the flags above; the run store, the cache and the job roots under $NSELF_CI_HOME.
// Outputs: the list of everything the tidy removes, and with --apply the free space before and
// after, and whether the target was reached or why not; exit 0, or 10 when --apply did not
// reach the target.
// Constraints: no new first-level subcommand: this wraps the `runs prune` handler. Without
// --apply nothing is removed. --apply refuses while a serve owns the store. Plain
// `runs prune` is unchanged.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/history"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
	"github.com/nself-org/plugins/free/ci/internal/selftest"
)

const tidyUsage = "nself ci runs prune --tidy [--apply] [--json]"

// tidyExitShort is the exit code of --apply when the free-space target was not reached.
const tidyExitShort = 10

func init() {
	prune := runsSubs["prune"] // registered by runs_history.go, which initialises first
	if prune == nil {
		panic("nself-ci: runs prune must register before cmd/runs_tidy.go")
	}
	runsSubs["prune"] = func(args []string) int {
		if rest, ok := cutFlag(args, "tidy"); ok {
			return runRunsTidy(rest)
		}
		return prune(args)
	}
}

// cutFlag removes a boolean --name flag from args and reports whether it was set.
func cutFlag(args []string, name string) ([]string, bool) {
	var rest []string
	on := false
	for _, a := range args {
		if n, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "="); strings.HasPrefix(a, "-") && n == name && (!hasVal || val == "true") {
			on = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, on
}

func runRunsTidy(args []string) int {
	fs := flag.NewFlagSet("runs prune --tidy", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "remove what the report lists (default: report only)")
	asJSON := fs.Bool("json", false, "print one v1 JSON envelope")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return usageErr(tidyUsage)
	}
	if *apply { // a serve owns the store; a second writer must not prune it
		if code, ok := refuseWhileServe("`runs prune --tidy --apply`"); ok {
			return code
		}
	}
	ctx := context.Background()
	rep, err := tidyReport(ctx, *apply)
	if err != nil {
		return exitForError(err)
	}
	if *asJSON {
		if err = output.WriteData(os.Stdout, "ci runs prune", rep, nil); err != nil {
			return exitForError(err)
		}
	} else {
		printTidy(rep)
	}
	if rep.Apply && !rep.Reached {
		return tidyExitShort
	}
	return 0
}

// tidyReport assembles the steps for this machine and runs them.
func tidyReport(ctx context.Context, apply bool) (selftest.Report, error) {
	home := sample.CIHome()
	root, _ := os.Getwd()
	eff, release, err := policyEffective(ctx, root)
	defer release()
	if err != nil {
		return selftest.Report{}, fmt.Errorf("tidy settings not read, nothing removed: %w", err)
	}
	cfg, err := selftest.TidyConfigFrom(&eff)
	if err != nil {
		return selftest.Report{}, err
	}
	pol, perr := history.LoadPolicy(os.Getenv("NSELF_CI_HOME"))
	if perr != nil {
		fmt.Fprintf(os.Stderr, "note: %s is unusable (%v); using the defaults\n", history.ConfigPath(os.Getenv("NSELF_CI_HOME")), perr)
	}
	var steps []selftest.Step
	if evidence.StoreExists(evidence.StorePath()) {
		fs := flag.NewFlagSet("tidy", flag.ContinueOnError)
		fs.Bool("json", false, "")
		fs.String("project", "tidy", "") // the viewer's project is not used: the tidy lists every entry
		env, err := openCache(fs)
		if err != nil {
			return selftest.Report{}, err
		}
		defer env.close()
		steps = append(steps, selftest.HistoryStep(env.st, os.Getenv("NSELF_CI_HOME"), pol, time.Now))
		if env.viewer.Role == authz.RoleAdmin { // the cache prune is an admin action
			es := entries.New(env.st, env.blobs, entries.Options{Home: records.StoreDir(os.Getenv("NSELF_CI_HOME")), Getenv: selftest.CacheCapEnv(cfg.CacheCapMB, os.Getenv)})
			steps = append(steps, selftest.CacheStep(env.st, es, 30*time.Second, time.Now))
		}
	}
	steps = append(steps, selftest.OrphanStep(filepath.Join(home, "work"), home, time.Now))
	t := selftest.Tidy{Steps: steps, TargetMB: cfg.FreeTargetMB, FreeMB: func() (int64, error) { return selftest.FreeMB(home) }}
	return t.Run(ctx, apply)
}

func printTidy(rep selftest.Report) {
	verb := "would remove"
	if rep.Apply {
		verb = "removed"
	}
	for _, it := range rep.Items {
		fmt.Printf("%s %-12s %s\n", verb, it.Step, it.ID)
	}
	fmt.Printf("%s %d item(s); %d MB free, target %d MB\n", verb, len(rep.Items), rep.FreeAfterMB, rep.TargetMB)
	for _, e := range rep.Errors {
		fmt.Println("error:", e)
	}
	switch {
	case rep.Apply && rep.Reached:
		fmt.Println("target reached")
	case rep.Why != "":
		fmt.Println(rep.Why)
	}
}
