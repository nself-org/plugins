// nself-ci — nSelf CI gate runner.
//
// Purpose: Run the gate suite for a repo (lint/test/build + gitleaks) and
//
//	optionally post a GitHub commit status so branch protection can require
//	the "nself-ci" check instead of billing-blocked GitHub Actions.
//
// Usage:
//
//	nself-ci [flags] [repo-root]
//	nself ci  (via nself CLI proxy)
//
// SPORT: PLUGINS-CI-000
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal"
)

func main() {
	var (
		skipStatus   = flag.Bool("no-status", false, "Run gates but do not post a GitHub commit status")
		skipGitleaks = flag.Bool("no-gitleaks", false, "Skip gitleaks secret scan")
		verbose      = flag.Bool("v", false, "Print each gate command before running")
		sha          = flag.String("sha", "", "Commit SHA to report on (default: HEAD)")
		owner        = flag.String("owner", "", "GitHub owner (default: from git remote)")
		repo         = flag.String("repo", "", "GitHub repo name (default: from git remote)")
		checkOnly    = flag.Bool("check", false, "Check mode: run gates, print result, exit 0/1. No status posted.")
	)
	flag.Parse()

	// repo-root is the optional positional argument.
	repoRoot := "."
	if flag.NArg() > 0 {
		repoRoot = flag.Arg(0)
	}
	// Env override.
	if v := os.Getenv("NSELF_CI_REPO"); v != "" && repoRoot == "." {
		repoRoot = v
	}
	if v := os.Getenv("NSELF_CI_SKIP_STATUS"); v == "1" {
		*skipStatus = true
	}

	cfg := internal.Config{
		RepoRoot:     repoRoot,
		SkipGitleaks: *skipGitleaks,
		Verbose:      *verbose,
	}

	// Determine SHA and remote before running gates (fail early on config errors).
	resolvedSHA := *sha
	if v := os.Getenv("NSELF_CI_SHA"); v != "" && resolvedSHA == "" {
		resolvedSHA = v
	}

	resolvedOwner := *owner
	resolvedRepo := *repo

	postStatus := !*skipStatus && !*checkOnly
	if postStatus {
		// Resolve SHA from git if not supplied.
		if resolvedSHA == "" {
			var err error
			resolvedSHA, err = internal.HeadSHA(repoRoot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot resolve HEAD SHA: %v\n", err)
				fmt.Fprintf(os.Stderr, "hint: pass --sha <sha> or use --no-status / --check\n")
				os.Exit(1)
			}
		}

		// Resolve owner/repo from git remote if not supplied.
		if resolvedOwner == "" || resolvedRepo == "" {
			o, r, err := internal.RepoOwnerName(repoRoot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot resolve GitHub remote: %v\n", err)
				fmt.Fprintf(os.Stderr, "hint: pass --owner and --repo, or use --no-status / --check\n")
				os.Exit(1)
			}
			if resolvedOwner == "" {
				resolvedOwner = o
			}
			if resolvedRepo == "" {
				resolvedRepo = r
			}
		}

		// Post a "pending" status before running so GitHub shows the check immediately.
		_ = internal.PostCommitStatus(internal.StatusConfig{
			Owner:       resolvedOwner,
			Repo:        resolvedRepo,
			SHA:         resolvedSHA,
			State:       "pending",
			Description: "nself-ci gate running…",
		})
	}

	// Run the gate suite.
	result, err := internal.Run(cfg)
	if err != nil {
		msg := fmt.Sprintf("gate error: %v", err)
		if postStatus {
			_ = internal.PostCommitStatus(internal.StatusConfig{
				Owner:       resolvedOwner,
				Repo:        resolvedRepo,
				SHA:         resolvedSHA,
				State:       "error",
				Description: msg,
			})
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Print results.
	printResults(result)

	// Post final commit status.
	if postStatus {
		state := "success"
		if !result.Passed {
			state = "failure"
		}
		if err := internal.PostCommitStatus(internal.StatusConfig{
			Owner:       resolvedOwner,
			Repo:        resolvedRepo,
			SHA:         resolvedSHA,
			State:       state,
			Description: result.Summary(),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not post commit status: %v\n", err)
		} else {
			fmt.Printf("\n✓ Posted nself-ci status %q to %s/%s@%s\n",
				state, resolvedOwner, resolvedRepo, resolvedSHA[:min(7, len(resolvedSHA))])
		}
	}

	if !result.Passed {
		os.Exit(1)
	}
}

// printResults prints a human-readable gate summary table.
func printResults(r *internal.Result) {
	fmt.Printf("\nnself-ci gate results — %s\n", r.RepoRoot)
	fmt.Printf("Stacks: %s\n", strings.Join(r.Stack, ", "))
	fmt.Println(strings.Repeat("─", 60))

	for _, g := range r.Gates {
		mark := "PASS"
		if !g.Passed {
			mark = "FAIL"
		}
		fmt.Printf("  %-30s  %s  (%s)\n", g.Name, mark, g.Elapsed.Round(1*1000*1000))
		if !g.Passed && g.Output != "" {
			// Indent output for readability.
			for _, line := range strings.SplitAfter(g.Output, "\n") {
				fmt.Print("    ", line)
			}
			fmt.Println()
		}
	}

	fmt.Println(strings.Repeat("─", 60))
	overall := "PASSED"
	if !r.Passed {
		overall = "FAILED"
	}
	fmt.Printf("  Overall: %s  (%s)\n\n", overall, r.Elapsed.Round(time.Second))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
