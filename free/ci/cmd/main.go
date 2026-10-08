// nself-ci — nSelf CI gate runner.
//
// Purpose: Run the gate suite for a repo (lint/test/build + gitleaks) and
//
//	optionally post a GitHub commit status so branch protection can require
//	the "nself-ci" check instead of billing-blocked GitHub Actions. The binary
//	also carries the core `nself ci` surface (build, forgejo, serve) ported
//	from the CLI (P7-CANON-09), so `nself ci …` and `nself-ci …` take the same
//	argv. Subcommand "run" discovers .ci.yaml plugin manifests and runs their stages.
//
// Usage:
//
//	nself-ci [flags] [repo-root]              — single-repo gate
//	nself-ci run [flags] [search-root]        — pipeline: discover .ci.yaml + run stages
//	nself-ci build --artifact android [dir]   — local signed-APK build + gh release upload
//	nself-ci forgejo [--url U] [--runner R]   — Forgejo server + runner health
//	nself-ci serve [flags]                    — webhook listener daemon (port 3845)
//	nself ci run --env local                  — via nself CLI proxy
//
// Gate flags: --check --no-status --no-gitleaks --filesystem -v/--verbose
// --sha --owner --repo --env --gateway (flags and the repo-root may be mixed).
//
// SPORT: PLUGINS-CI-000
package main

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/compatcheck"
)

func main() {
	if err := compatcheck.Check(os.Getenv, compatcheck.RequiresNself); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// compat.V15(P7-CI-30): v1.4 gate dispatch -> v1.5 fabric dispatch.
	if compat.V15() {
		os.Exit(runV15(os.Args[1:]))
	}
	os.Exit(run(os.Args[1:]))
}

// run dispatches argv and returns the exit code. A bare -h/--help in front of
// a command that has captured core help prints that text; the first token
// that is not a flag selects a registered subcommand, otherwise the argv is
// the single-repo gate.
func run(args []string) int {
	if h, key, rest, ok := resolve(args); ok {
		if wantsHelp(rest) {
			if printHelp(key) {
				return 0
			}
		}
		return h(rest)
	}
	if wantsHelp(args) && printHelp("") {
		return 0
	}
	return gateCmd(args)
}
