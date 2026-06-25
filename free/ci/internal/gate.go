// Package internal implements the nself-ci gate runner.
//
// Purpose: Detect repo stack (Go/Node/Flutter) and run the appropriate
//
//	lint+test+build checks, plus a gitleaks secret scan.
//
// Inputs:  repoRoot string, cfg Config
// Outputs: Result (passed bool, gate results, log)
// Constraints: No network calls; pure subprocess execution.
// SPORT: PLUGINS-CI-001
package internal

import (
	"time"
)

// Config controls gate execution.
type Config struct {
	// RepoRoot is the directory to gate. Defaults to cwd.
	RepoRoot string
	// Timeout for each individual gate step (seconds). Default 120.
	StepTimeout int
	// SkipGitleaks skips the secret scan (useful in local dev without gitleaks binary).
	SkipGitleaks bool
	// Verbose prints each command before running it.
	Verbose bool
	// GatewayBase is the base URL for the gateway routing check stage.
	// If non-empty, a gateway-routing-check stage is appended after all stack gates.
	// Example: "http://167.235.233.65:3761"
	// SPORT: PLUGINS-CI-005
	GatewayBase string
	// EvalGateURL is the nself-eval-gate plugin base URL.
	// If non-empty, an eval step is appended: `nself ci eval --all --output json`.
	// On CI, reads NSELF_EVAL_GATE_URL env var; omit to skip eval gate.
	// Example: "http://localhost:3770"
	// SPORT: PLUGINS-CI-EVAL-001
	EvalGateURL string
	// EvalTierPromotion signals that a tier promotion is in-flight for this CI run.
	// When true and eval passed=false, CI exits non-zero regardless of other gates.
	EvalTierPromotion bool
}

// GateResult holds the outcome of a single gate step.
type GateResult struct {
	Name    string
	Passed  bool
	Output  string
	Elapsed time.Duration
}

// Result is the overall gate run result.
type Result struct {
	RepoRoot string
	Stack    []string
	Gates    []GateResult
	Passed   bool
	Elapsed  time.Duration
}

// Summary returns a one-line description for use in a GitHub commit status.
