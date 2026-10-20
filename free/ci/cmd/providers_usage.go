package main

// Purpose: `nself ci providers usage import [--repo dir] [--json]` (P7-HOST-10, Epic D12/D14): import
// the GitHub enhanced-billing usage of the account in providers.github.billing_account into the
// SCHED-07 ledger, where `nself ci policy usage` shows it as `known`.
// Inputs:  argv after the key; env NSELF_CI_HOME (the run store), NSELF_CI_REPO; the operator's
//          providers.github.* keys and the credential the HOST-16 runtime resolves.
// Outputs: `disabled` when providers.github.billing_import is not true (no API call, exit 0); else a
//          one-line summary or one v1 JSON envelope; exit 0, 1 (usage, E700, E709 and other
//          provider refusals) or 2 (infrastructure).
// Constraints: V15 only; writes the store only through economics.RecordUsage; requests only the
//          non-summary billing usage endpoint (see internal/providers/github/usage); never asks for
//          or refreshes a token scope. Live use waits for the owner to name the token scope.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const providersUsageUsage = "nself ci providers usage import [--repo dir] [--json]"

type providersUsageFlags struct {
	json bool
	repo string
}

func newProvidersUsageFlagSet(f *providersUsageFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("providers usage import", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	return fs
}

func init() {
	registerV15("providers usage import", subcommandEntry{V15Only: true, Handler: runProvidersUsageImport,
		Flags: func() *flag.FlagSet { return newProvidersUsageFlagSet(&providersUsageFlags{}) }})
}

func runProvidersUsageImport(args []string) int {
	var f providersUsageFlags
	fs := newProvidersUsageFlagSet(&f)
	fs.SetOutput(os.Stderr)
	if pos, err := parseMixed(fs, args); err != nil || len(pos) > 0 {
		return usageErr(providersUsageUsage)
	}
	root := firstOf(f.repo, os.Getenv("NSELF_CI_REPO"))
	var err error
	if root == "" {
		root, err = os.Getwd()
	} else {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return exitForError(err)
	}
	ctx := context.Background()
	res, err := importUsage(ctx, root)
	if err != nil {
		return usageImportFail(f, err)
	}
	if f.json {
		if err = output.WriteData(os.Stdout, "ci providers usage import", res, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if res.Disabled {
		fmt.Println("disabled")
		return 0
	}
	fmt.Printf("imported %d usage lines for %s (%s to %s)\n", res.Rows, res.Account, res.From, res.To)
	return 0
}

// importUsage resolves the operator policy of root and runs one import against the run store. With
// the import off it returns before the store is opened or any credential is read.
func importUsage(ctx context.Context, root string) (usage.Result, error) {
	res := policy.Resolver{Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return usage.Result{}, err
	}
	imp, err := usage.FromPolicy(ctx, &got.Effective)
	if err != nil {
		return usage.Result{}, err
	}
	if !imp.Settings.Import {
		return imp.Run(ctx)
	}
	st, err := openHistory(true)
	if err != nil {
		return usage.Result{}, err
	}
	defer func() { _ = st.Close() }()
	ctx = economics.WithEnv(ctx, economics.Env{Store: st, Now: time.Now, Coordinator: "cli"})
	return imp.Run(ctx)
}

// usageImportFail prints err and returns the exit code: a provider refusal is the caller's to fix (1).
func usageImportFail(f providersUsageFlags, err error) int {
	var pe *providers.Error
	if !errors.As(err, &pe) {
		return exitForError(err)
	}
	if f.json {
		_ = output.WriteError(os.Stdout, "ci providers usage import", output.ErrorDetail{Code: pe.Code, Message: err.Error(), Cause: pe.Reason, ExitCode: 1, Class: output.ClassUser}, nil)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%s: usage import failed (%s)\n", pe.Code, pe.Reason)
	return 1
}
