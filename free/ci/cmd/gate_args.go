package main

// Purpose: parse the single-repo gate argv and translate the core `nself ci`
// flags to gate options. The table below is the one place that maps a core
// flag (and its legacy spelling) to an option; gate_args_test.go pins it.
// Inputs:  argv after the program name: flags and one repo-root, in any order
// (cobra allows `nself ci /repo --check`; the legacy flag package did not).
// Outputs: gateOpts; help detection; core console framing.
// Constraints: every argv the deployed 1.4.x buildCIArgs produces keeps its
// exact meaning (EPIC D11): --no-status, --no-gitleaks, --filesystem, -v,
// --sha X, --owner X, --repo X, then an absolute repo root.
//
// Core flag translation table (core flag -> option):
//   --check            -> check (no status posted; also turns on core framing)
//   --no-status        -> skipStatus (alias of --check in core, and in the gate)
//   --no-gitleaks      -> skipGitleaks
//   --filesystem       -> forceFilesystem
//   -v, --verbose      -> verbose
//   --sha S            -> sha
//   --owner O          -> owner
//   --repo R           -> repo
//   [repo-root]        -> repoRoot (made absolute, as core does)
//   --env, --gateway   -> gateway routing (existing plugin flags, unchanged)

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/clui"
	"github.com/nself-org/plugins/free/ci/internal/surface"
)

// gateOpts is the parsed single-repo gate request.
type gateOpts struct {
	check           bool
	skipStatus      bool
	skipGitleaks    bool
	forceFilesystem bool
	verbose         bool
	sha             string
	owner           string
	repo            string
	env             string
	gateway         string
	repoRoot        string
}

// postStatus reports whether a GitHub commit status is posted.
func (o gateOpts) postStatus() bool { return !o.skipStatus && !o.check }

// frame reports whether the core console framing is printed. Core's `nself ci`
// printed the section header and "Error: gate failed" itself and then ran the
// binary; the 1.4.x proxy argv never carries --check (core maps it to
// --no-status), so --check is the one signal that the caller wants core's
// framing and the frozen proxy output stays byte-identical.
func (o gateOpts) frame() bool { return o.check }

// parseGateArgs parses argv into gateOpts. On a flag error it follows eh
// (flag.ExitOnError in the binary, flag.ContinueOnError in tests).
func parseGateArgs(args []string, eh flag.ErrorHandling) (gateOpts, error) {
	var o gateOpts
	fs := flag.NewFlagSet("nself-ci", eh)
	fs.BoolVar(&o.skipStatus, "no-status", false, "Run gates but do not post a GitHub commit status")
	fs.BoolVar(&o.skipGitleaks, "no-gitleaks", false, "Skip gitleaks secret scan")
	fs.BoolVar(&o.verbose, "v", false, "Print each gate command before running")
	fs.BoolVar(&o.verbose, "verbose", false, "Alias for -v (core spelling)")
	fs.StringVar(&o.sha, "sha", "", "Commit SHA to report on (default: HEAD)")
	fs.StringVar(&o.owner, "owner", "", "GitHub owner (default: from git remote)")
	fs.StringVar(&o.repo, "repo", "", "GitHub repo name (default: from git remote)")
	fs.BoolVar(&o.check, "check", false, "Check mode: run gates, print result, exit 0/1. No status posted.")
	fs.StringVar(&o.env, "env", "", "Target environment: local (SPORT: PLUGINS-CI-005)")
	fs.StringVar(&o.gateway, "gateway", "", "Explicit gateway base URL override (e.g. http://host:3761)")
	fs.BoolVar(&o.forceFilesystem, "filesystem", false, "Force gitleaks filesystem scan (--no-git) even inside a git checkout; opt-in for non-checkout source trees such as an exported tarball")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return o, err
	}
	o.repoRoot = "."
	if len(pos) > 0 {
		o.repoRoot = pos[0]
	}
	// Env fallbacks, unchanged from the legacy gate (read before making the root absolute).
	if v := os.Getenv("NSELF_CI_REPO"); v != "" && o.repoRoot == "." {
		o.repoRoot = v
	}
	if os.Getenv("NSELF_CI_SKIP_STATUS") == "1" {
		o.skipStatus = true
	}
	if abs, aerr := filepath.Abs(o.repoRoot); aerr == nil {
		o.repoRoot = abs
	}
	return o, nil
}

// parseInterspersed parses flags that may follow positional arguments and
// returns the positionals in order.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// wantsHelp reports whether argv asks for help (before a "--" terminator).
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" || a == "-help" {
			return true
		}
	}
	return false
}

// printHelp prints the captured core help for key and reports whether there was any.
func printHelp(key string) bool {
	txt, ok := surface.Help(key)
	if !ok {
		return false
	}
	fmt.Print(txt)
	return true
}

// frameHeader prints the lines core printed before running the gate.
func frameHeader(o gateOpts) {
	clui.Section("nself-ci gate")
	if o.postStatus() {
		clui.Info(fmt.Sprintf("repo: %s", o.repoRoot))
	} else {
		clui.Info(fmt.Sprintf("repo: %s (check mode — no status posted)", o.repoRoot))
	}
}

// frameFailure prints the error line core printed when the gate exited 1.
func frameFailure() {
	fmt.Fprintln(os.Stderr, "Error: gate failed")
}
