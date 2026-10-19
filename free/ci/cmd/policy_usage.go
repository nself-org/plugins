package main

// Purpose: `nself ci policy usage [--repo dir] [--json]` (P7-SCHED-07, Epic D11,
// X18): the usage ledger. Budgets with what is reserved in the current UTC
// windows, each provider's declared rate, ceiling, imported consumption, local
// ledger and allowance, the latest attempts with estimate and actual, and the
// recorded spend approvals. Every money figure prints with its confidence
// (known, computed, estimated or unknown) and where it came from.
// Inputs:  argv after the key; env NSELF_CI_HOME (the run store), NSELF_CI_REPO.
// Outputs: a table, or one v1 JSON envelope with --json; exit 0, 1 (usage) or 2
// (infrastructure).
// Constraints: V15 only; read only; calls no billing or pricing API; an
// allowance or an estimate is never printed without its confidence; no store on
// this machine is an empty ledger under the declared policy.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const policyUsageUsage = "nself ci policy usage [--repo dir] [--json]"

type usageFlags struct {
	json bool
	repo string
}

func newUsageFlagSet(f *usageFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("policy usage", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	return fs
}

func init() {
	registerV15("policy usage", subcommandEntry{V15Only: true, Handler: runPolicyUsage,
		Flags: func() *flag.FlagSet { return newUsageFlagSet(&usageFlags{}) }})
}

func runPolicyUsage(args []string) int {
	var f usageFlags
	fs := newUsageFlagSet(&f)
	fs.SetOutput(os.Stderr)
	pos, err := parseMixed(fs, args)
	if err != nil || len(pos) > 0 {
		return usageErr(policyUsageUsage)
	}
	ctx := context.Background()
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	root := firstOf(f.repo, os.Getenv("NSELF_CI_REPO"))
	if root == "" {
		root, err = os.Getwd()
	} else {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return exitForError(err)
	}
	rep, err := usageReport(ctx, root, st, time.Now())
	if err != nil {
		return exitForError(err)
	}
	if f.json {
		if err = output.WriteData(os.Stdout, "ci policy usage", rep, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	printUsage(rep)
	return 0
}

// usageReport resolves the declared spend policy of root and reads the ledger of st (nil: empty).
func usageReport(ctx context.Context, root string, st *store.Store, now time.Time) (economics.Report, error) {
	res := policy.Resolver{Run: policy.Runner(report.Capture)}
	if st != nil {
		res.Store = st
	}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return economics.Report{}, err
	}
	sp, err := schedpolicy.Spend(&got)
	if err != nil {
		return economics.Report{}, err
	}
	if st == nil {
		return economics.EmptyReport(sp, now), nil
	}
	return economics.BuildReport(ctx, st, sp, now)
}

func fig(f economics.Figure) string {
	return fmt.Sprintf("%d micro (%s: %s)", f.Micro, f.Confidence, f.Source)
}

func printUsage(r economics.Report) {
	fmt.Printf("window %s / %s UTC, currency %s, paid spend %s\n", r.Day, r.Month, r.Currency, map[bool]string{true: "enabled", false: "disabled"}[r.PaidEnabled])
	for _, b := range r.Budgets {
		line := fmt.Sprintf("budget %-16s limit %s", b.Scope, fig(b.Limit))
		if b.Reserved != nil {
			line += "; reserved " + fig(*b.Reserved)
		}
		fmt.Println(line)
	}
	for _, p := range r.Providers {
		fmt.Printf("provider %-14s class %-9s local %s\n", p.Provider, p.Class, fig(p.Local))
		if p.Imported != nil {
			fmt.Printf("  imported %s\n", fig(*p.Imported))
		}
		if p.Allowance != nil {
			fmt.Printf("  allowance %s, consumption known: %v\n", fig(*p.Allowance), p.AllowanceKnown)
		}
	}
	for _, a := range r.Recent {
		line := fmt.Sprintf("attempt %-28s %-12s %-9s estimate %s", a.Attempt, a.Provider, a.Class, fig(a.Estimate))
		if a.Actual != nil {
			line += "; actual " + fig(*a.Actual)
		}
		fmt.Println(line)
	}
	for _, a := range r.Approvals {
		fmt.Printf("approval run %s by %s: %s\n", a.Run, a.Actor, fig(a.Estimate))
	}
}
