package cli

// Purpose: `nself eval` — run a named eval suite against the nself-eval-gate
//   plugin, or validate an eval-set YAML without running it. Port of core
//   `nself ci eval` (cli internal/cmd/ci/eval.go).
// Inputs:  --suite, --validate-only, --output, --k, --dry-run, --repo, --tier,
//   --eval-url, --source-account (+ reserved --all, --fail-fast, --no-cache).
// Outputs: eval-results.json under {repo}/.nself-ci/artifacts/; the formatted
//   result on stdout; progress and errors on stderr (as core's cobra Printf did).
// Constraints: exit codes 0=passed, 1=below threshold, 2=validation error,
//   3=precondition not met (BGE-M3/gateway unavailable). Frozen contract.
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Exit codes: the documented contract for CI wrappers.
const (
	exitPassed        = 0
	exitBelowTreshold = 1
	exitValidation    = 2
	exitPrecondition  = 3
)

var evalFlags = []flagDef{
	{"suite", kindString, ""},
	{"output", kindString, FormatSummary},
	{"repo", kindString, "."},
	{"tier", kindString, ""},
	{"eval-url", kindString, "http://localhost:3770"},
	{"source-account", kindString, "primary"},
	{"k", kindInt, "3"},
	{"validate-only", kindBool, "false"},
	{"dry-run", kindBool, "false"},
	{"all", kindBool, "false"},
	{"fail-fast", kindBool, "false"},
	{"no-cache", kindBool, "false"},
}

// io carries the process streams so tests can capture them.
type streams struct{ out, err io.Writer }

func (s streams) errf(format string, a ...any) { _, _ = fmt.Fprintf(s.err, format, a...) }

func runEval(args []string, s streams) int {
	p, err := parseArgs(evalFlags, args)
	if err != nil {
		s.errf("Error: %v\n", err)
		return exitBelowTreshold
	}
	if p.help {
		_, _ = io.WriteString(s.out, evalHelp)
		return exitPassed
	}
	suite := p.str("suite")
	// Exit codes here are a documented contract for CI wrappers.
	if suite == "" {
		s.errf("error: --suite is required\n")
		return exitValidation
	}
	repoAbs, err := filepath.Abs(p.str("repo"))
	if err != nil {
		s.errf("Error: resolve repo root: %v\n", err)
		return exitBelowTreshold
	}
	client := &Client{BaseURL: p.str("eval-url"), SourceAccountID: p.str("source-account")}
	ctx := context.Background()
	if p.boolean("validate-only") {
		return runValidateOnly(ctx, s, client, suite, repoAbs)
	}
	if p.boolean("dry-run") {
		s.errf("dry-run: would run suite=%s via %s\n", suite, p.str("eval-url"))
		s.errf("dry-run: artifact output → %s/.nself-ci/artifacts/eval-results.json\n", repoAbs)
		return exitPassed
	}
	return runSuite(ctx, s, client, suite, p.str("output"), repoAbs)
}

// runValidateOnly validates eval-set YAML without executing the suite. It reads
// the eval-set from {repo}/evals/{suite}.yaml, {repo}/.nself-ci/evals/{suite}.yaml
// or {repo}/{suite}.yaml; with none present it submits empty content, so the
// plugin returns its validation errors.
func runValidateOnly(ctx context.Context, s streams, client *Client, suite, repoRoot string) int {
	candidates := []string{
		filepath.Join(repoRoot, "evals", suite+".yaml"),
		filepath.Join(repoRoot, ".nself-ci", "evals", suite+".yaml"),
		filepath.Join(repoRoot, suite+".yaml"),
	}
	var content []byte
	var readErr error
	for _, path := range candidates {
		if content, readErr = os.ReadFile(path); readErr == nil {
			break
		}
	}
	if readErr != nil {
		content = []byte{}
	}
	result, err := client.ValidateYAML(ctx, content)
	if err != nil {
		s.errf("error: %v\n", err)
		return exitValidation
	}
	if result.Valid {
		s.errf("valid: eval-set schema OK for suite=%s\n", suite)
		return exitPassed
	}
	WriteValidationErrors(s.out, result.Errors)
	return exitValidation
}

// runSuite triggers the eval run and polls for completion.
func runSuite(ctx context.Context, s streams, client *Client, suite, format, repoRoot string) int {
	s.errf("running eval suite: %s\n", suite)
	queued, err := client.RunSuite(ctx, suite)
	if err != nil {
		s.errf("error: trigger eval: %v\n", err)
		return exitBelowTreshold
	}
	s.errf("run queued: %s (polling...)\n", queued.RunID)
	result, err := client.WaitForRun(ctx, queued.RunID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPreconditionNotMet):
			s.errf("error: precondition not met — BGE-M3 or AI gateway unavailable\n")
			return exitPrecondition
		case errors.Is(err, ErrPollTimeout):
			s.errf("error: run did not complete within 10 minutes\n")
		default:
			s.errf("error: poll run: %v\n", err)
		}
		return exitBelowTreshold
	}
	if werr := WriteResult(s.out, result, format, repoRoot); werr != nil {
		s.errf("warning: output error: %v\n", werr)
	}
	if !result.Passed {
		return exitBelowTreshold
	}
	return exitPassed
}
