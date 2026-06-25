package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Purpose: Eval gate step for nself-ci pipeline.
//   Invokes `nself ci eval --all --repo {root} --output json --eval-url {evalURL}`.
//   Writes eval-results.json artifact to {root}/.nself-ci/artifacts/.
//   CI fails when eval passed=false AND tierPromotion=true (tier in-flight).
//   When tierPromotion=false, eval failure is a warning (non-blocking) so eval suites
//   can be added before thresholds are enforced.
// Inputs:  repoRoot, evalURL, tierPromotion, timeout, verbose.
// Outputs: GateResult with passed=true/false + formatted output.
// Constraints: Uses `nself` binary on PATH. Non-zero exit from `nself ci eval`
//   sets passed=false. EvalGateURL must be reachable (plugin running).
// SPORT: PLUGINS-CI-EVAL-001

// evalResultSummary is a minimal decode of eval-results.json to determine passed flag.
type evalResultSummary struct {
	Passed     bool   `json:"passed"`
	PassRate   float64 `json:"pass_rate"`
	SuiteScore float64 `json:"suite_score"`
	Status     string `json:"status"`
	SuiteSlug  string `json:"suite_slug"`
}

// runEvalGateStep runs `nself ci eval --all --output json --eval-url evalURL --repo root`.
// Purpose: Integrate eval-gate quality check into nself-ci pipeline.
// Inputs:  repoRoot, evalURL (nself-eval-gate base), tierPromotion, timeout, verbose.
// Outputs: GateResult; Passed=false blocks CI only when tierPromotion=true.
// Constraints: Reads eval-results.json from artifacts dir after run to confirm artifact written.
func runEvalGateStep(repoRoot, evalURL string, tierPromotion bool, timeout int, verbose bool) GateResult {
	start := time.Now()
	gr := GateResult{Name: "eval:gate"}

	// Build command: nself ci eval --all --output json --eval-url <url> --repo <root>
	args := []string{
		"ci", "eval",
		"--all",
		"--output", "json",
		"--eval-url", evalURL,
		"--repo", repoRoot,
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  [eval:gate] nself %s\n", strings.Join(args, " "))
	}

	cmd := exec.Command("nself", args...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	gr.Output = strings.TrimSpace(string(out))
	gr.Elapsed = time.Since(start)

	// Determine eval pass/fail from the written artifact (more reliable than exit code
	// since `nself ci eval` may exit 1 for below-threshold which is recoverable).
	artifactPath := filepath.Join(repoRoot, ".nself-ci", "artifacts", "eval-results.json")
	evalPassed := readEvalArtifactPassed(artifactPath)

	if err != nil {
		if !evalPassed {
			if tierPromotion {
				// Hard failure: tier promotion in-flight, eval must pass.
				gr.Passed = false
				gr.Output = fmt.Sprintf("eval gate FAILED (tier promotion in-flight): %s", gr.Output)
			} else {
				// Advisory: eval below threshold but no tier promotion — warn only.
				gr.Passed = true
				gr.Output = fmt.Sprintf("[advisory] eval gate below threshold (no tier promotion in-flight): %s", gr.Output)
			}
		} else {
			// Command failed but artifact says passed — treat as infrastructure error.
			gr.Passed = false
			gr.Output = fmt.Sprintf("eval gate infrastructure error: %s", gr.Output)
		}
	} else {
		gr.Passed = true
	}

	return gr
}

// readEvalArtifactPassed parses eval-results.json and returns the passed field.
// Returns false if the file is absent or unparseable.
func readEvalArtifactPassed(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var summary evalResultSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return false
	}
	return summary.Passed
}
