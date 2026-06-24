// nself-ci — nSelf CI gate runner.
//
// Purpose: Run the gate suite for a repo (lint/test/build + gitleaks) and
//
//	optionally post a GitHub commit status so branch protection can require
//	the "nself-ci" check instead of billing-blocked GitHub Actions.
//	Subcommand "run" discovers .ci.yaml plugin manifests and runs their stages.
//
// Usage:
//
//	nself-ci [flags] [repo-root]              — single-repo gate
//	nself-ci run [flags] [search-root]        — pipeline: discover .ci.yaml + run stages
//	nself ci run --env staging                — via nself CLI proxy
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

// gatewayBaseForEnv maps --env values to gateway base URLs.
// Staging targets 167.235.233.65:3761 (E7 completion gate).
// NEVER add production IP here (5.75.235.42 is deny-listed per destructive-deny-list.md).
var gatewayBaseForEnv = map[string]string{
	"staging": "http://167.235.233.65:3761",
	"local":   "http://127.0.0.1:3761",
}

func main() {
	// Check for "run" subcommand as first non-flag argument.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "run" {
		runPipelineCmd(args[1:])
		return
	}

	// Default: single-repo gate (legacy behaviour, unchanged).
	runSingleRepoCmd(args)
}

// runPipelineCmd implements "nself-ci run [flags] [search-root]".
// Discovers .ci.yaml manifests under search-root and runs all stages in canonical order.
// Adds a gateway routing check stage when --env or --gateway is provided.
func runPipelineCmd(rawArgs []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var (
		env        = fs.String("env", "", "Target environment: staging|local — sets gateway base URL")
		gatewayURL = fs.String("gateway", "", "Explicit gateway base URL (e.g. http://host:3761)")
		verbose    = fs.Bool("v", false, "Print each command before running")
		timeout    = fs.Int("timeout", 300, "Per-step timeout in seconds")
	)
	_ = fs.Parse(rawArgs)

	searchRoot := "."
	if fs.NArg() > 0 {
		searchRoot = fs.Arg(0)
	}
	if v := os.Getenv("NSELF_CI_SEARCH_ROOT"); v != "" && searchRoot == "." {
		searchRoot = v
	}

	// Resolve gateway base URL.
	gatewayBase := *gatewayURL
	if gatewayBase == "" && *env != "" {
		base, ok := gatewayBaseForEnv[*env]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown --env %q (valid: staging, local)\n", *env)
			os.Exit(1)
		}
		gatewayBase = base
	}
	if v := os.Getenv("NSELF_CI_GATEWAY"); v != "" && gatewayBase == "" {
		gatewayBase = v
	}

	fmt.Printf("nself-ci pipeline — searching %s\n", searchRoot)
	if gatewayBase != "" {
		fmt.Printf("gateway routing check: %s\n", gatewayBase)
	}
	fmt.Println(strings.Repeat("─", 60))

	gates, err := internal.DiscoverAndRunPipeline(searchRoot, gatewayBase, *timeout, *verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if len(gates) == 0 {
		fmt.Println("No .ci.yaml manifests found under", searchRoot)
		os.Exit(0)
	}

	passed := printPipelineResults(gates)
	if !passed {
		os.Exit(1)
	}
}

// printPipelineResults prints a gate table and returns true if all passed.
func printPipelineResults(gates []internal.GateResult) bool {
	allPassed := true
	for _, g := range gates {
		mark := "PASS"
		if !g.Passed {
			mark = "FAIL"
			allPassed = false
		}
		fmt.Printf("  %-45s  %s  (%s)\n", g.Name, mark, g.Elapsed.Round(time.Millisecond))
		if !g.Passed && g.Output != "" {
			for _, line := range strings.SplitAfter(g.Output, "\n") {
				fmt.Print("    ", line)
			}
			fmt.Println()
		}
	}
	fmt.Println(strings.Repeat("─", 60))
	overall := "PASSED"
	if !allPassed {
		overall = "FAILED"
	}
	fmt.Printf("  Pipeline: %s\n\n", overall)
	return allPassed
}

// runSingleRepoCmd implements the original gate behaviour for a single repo root.
func runSingleRepoCmd(rawArgs []string) {
	fs := flag.NewFlagSet("nself-ci", flag.ExitOnError)
	var (
		skipStatus   = fs.Bool("no-status", false, "Run gates but do not post a GitHub commit status")
		skipGitleaks = fs.Bool("no-gitleaks", false, "Skip gitleaks secret scan")
		verbose      = fs.Bool("v", false, "Print each gate command before running")
		sha          = fs.String("sha", "", "Commit SHA to report on (default: HEAD)")
		owner        = fs.String("owner", "", "GitHub owner (default: from git remote)")
		repo         = fs.String("repo", "", "GitHub repo name (default: from git remote)")
		checkOnly    = fs.Bool("check", false, "Check mode: run gates, print result, exit 0/1. No status posted.")
		env          = fs.String("env", "", "Target environment for gateway routing check: staging|local (SPORT: PLUGINS-CI-005)")
		gatewayURL   = fs.String("gateway", "", "Explicit gateway base URL override (e.g. http://host:3761)")
	)
	_ = fs.Parse(rawArgs)

	// repo-root is the optional positional argument.
	repoRoot := "."
	if fs.NArg() > 0 {
		repoRoot = fs.Arg(0)
	}
	if v := os.Getenv("NSELF_CI_REPO"); v != "" && repoRoot == "." {
		repoRoot = v
	}
	if v := os.Getenv("NSELF_CI_SKIP_STATUS"); v == "1" {
		*skipStatus = true
	}

	// Resolve gateway base URL.
	gatewayBase := *gatewayURL
	if gatewayBase == "" && *env != "" {
		if base, ok := gatewayBaseForEnv[*env]; ok {
			gatewayBase = base
		} else {
			fmt.Fprintf(os.Stderr, "error: unknown --env %q (valid: staging, local)\n", *env)
			os.Exit(1)
		}
	}
	if v := os.Getenv("NSELF_CI_GATEWAY"); v != "" && gatewayBase == "" {
		gatewayBase = v
	}

	cfg := internal.Config{
		RepoRoot:     repoRoot,
		SkipGitleaks: *skipGitleaks,
		Verbose:      *verbose,
		GatewayBase:  gatewayBase,
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
		if resolvedSHA == "" {
			var err error
			resolvedSHA, err = internal.HeadSHA(repoRoot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot resolve HEAD SHA: %v\n", err)
				fmt.Fprintf(os.Stderr, "hint: pass --sha <sha> or use --no-status / --check\n")
				os.Exit(1)
			}
		}

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

		_ = internal.PostCommitStatus(internal.StatusConfig{
			Owner:       resolvedOwner,
			Repo:        resolvedRepo,
			SHA:         resolvedSHA,
			State:       "pending",
			Description: "nself-ci gate running…",
		})
	}

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

	printResults(result)

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
		fmt.Printf("  %-30s  %s  (%s)\n", g.Name, mark, g.Elapsed.Round(time.Millisecond))
		if !g.Passed && g.Output != "" {
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
