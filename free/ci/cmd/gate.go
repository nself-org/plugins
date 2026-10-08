package main

// Purpose: the single-repo gate command: run lint/test/build + gitleaks for a
// repo root and optionally post the nself-ci GitHub commit status.
// Inputs:  argv (see gate_args.go).
// Outputs: the gate table on stdout; commit status via gh; exit code 0/1.
// Constraints: behaviour unchanged from the pre-port main.go (PLUGINS-CI-000);
// the only additions are core argv acceptance and core framing under --check.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/legacy"
)

// gateCmd implements "nself-ci [flags] [repo-root]" and returns the exit code.
func gateCmd(args []string) int {
	o, err := parseGateArgs(args, flag.ExitOnError)
	if err != nil {
		return 2
	}
	if o.frame() {
		frameHeader(o)
	}
	rc := runGate(o)
	if o.frame() && rc == 1 {
		frameFailure()
	}
	return rc
}

// runGate runs the gate for o and returns the exit code.
func runGate(o gateOpts) int {
	// Resolve gateway base URL.
	gatewayBase, err := resolveGatewayBase(o.env, o.gateway, os.Getenv("NSELF_CI_GATEWAY"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	cfg := legacy.Config{
		RepoRoot:        o.repoRoot,
		SkipGitleaks:    o.skipGitleaks,
		Verbose:         o.verbose,
		GatewayBase:     gatewayBase,
		ForceFilesystem: o.forceFilesystem,
	}

	// Determine SHA and remote before running gates (fail early on config errors).
	resolvedSHA := o.sha
	if v := os.Getenv("NSELF_CI_SHA"); v != "" && resolvedSHA == "" {
		resolvedSHA = v
	}
	resolvedOwner, resolvedRepo := o.owner, o.repo

	post := o.postStatus()
	if post {
		var rc int
		resolvedSHA, resolvedOwner, resolvedRepo, rc = resolveStatusTarget(o, resolvedSHA, resolvedOwner, resolvedRepo)
		if rc != 0 {
			return rc
		}
		_ = legacy.PostCommitStatus(legacy.StatusConfig{
			Owner:       resolvedOwner,
			Repo:        resolvedRepo,
			SHA:         resolvedSHA,
			State:       "pending",
			Description: "nself-ci gate running…",
		})
	}

	result, err := legacy.Run(cfg)
	if err != nil {
		if post {
			_ = legacy.PostCommitStatus(legacy.StatusConfig{
				Owner:       resolvedOwner,
				Repo:        resolvedRepo,
				SHA:         resolvedSHA,
				State:       "error",
				Description: fmt.Sprintf("gate error: %v", err),
			})
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	printResults(result)

	if post {
		state := "success"
		if !result.Passed {
			state = "failure"
		}
		if err := legacy.PostCommitStatus(legacy.StatusConfig{
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
		return 1
	}
	return 0
}

// resolveStatusTarget fills the SHA, owner and repo a status post needs and
// returns a non-zero exit code (after printing core's messages) when it cannot.
func resolveStatusTarget(o gateOpts, sha, owner, repo string) (string, string, string, int) {
	if sha == "" {
		var err error
		sha, err = legacy.HeadSHA(o.repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot resolve HEAD SHA: %v\n", err)
			fmt.Fprintf(os.Stderr, "hint: pass --sha <sha> or use --no-status / --check\n")
			return "", "", "", 1
		}
	}
	if owner == "" || repo == "" {
		ro, rr, err := legacy.RepoOwnerName(o.repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot resolve GitHub remote: %v\n", err)
			fmt.Fprintf(os.Stderr, "hint: pass --owner and --repo, or use --no-status / --check\n")
			return "", "", "", 1
		}
		if owner == "" {
			owner = ro
		}
		if repo == "" {
			repo = rr
		}
	}
	return sha, owner, repo, 0
}

// printResults prints a human-readable gate summary table.
func printResults(r *legacy.Result) {
	fmt.Printf("\nnself-ci gate results — %s\n", r.RepoRoot)
	fmt.Printf("Stacks: %s\n", strings.Join(r.Stack, ", "))
	fmt.Println(strings.Repeat("─", 60))

	for _, g := range r.Gates {
		mark := "PASS"
		switch {
		case g.Skipped:
			mark = "SKIP"
		case !g.Passed:
			mark = "FAIL"
		}
		fmt.Printf("  %-30s  %s  (%s)\n", g.Name, mark, g.Elapsed.Round(time.Millisecond))
		// Always show WHY for a skip or a failure — a silent SKIP/PASS line
		// is exactly the shape of the G-015 bug this gate now refuses to be.
		if (g.Skipped || !g.Passed) && g.Output != "" {
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
