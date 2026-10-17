package legacy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runEnvLiveKeyGate runs the repository's optional env gate before gitleaks.
// Inputs: repository root and verbose flag. Outputs: nil when absent, or verdict.
// Constraints: exit 1 means findings; exit 2 or inability to run is a gate error.
// SPORT: D-0471.
func runEnvLiveKeyGate(root string, verbose bool) *GateResult {
	const path = "scripts/ci/env-live-key-gate.sh"
	const name = "secrets:env-live-key"
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return &GateResult{Name: name, Output: fmt.Sprintf("gate error: %v", err)}
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return &GateResult{Name: name, Output: fmt.Sprintf("gate error: %v", err)}
	}
	if !info.Mode().IsRegular() {
		return &GateResult{Name: name, Output: "gate error: env live-key script is not a regular file"}
	}
	gr, err := runRepoScriptResult(name, root, path, verbose)
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			gr.Output = fmt.Sprintf("gate error: %v\n%s", err, gr.Output)
		}
		if gr.Skipped {
			gr.Output = fmt.Sprintf("gate error: cannot run env live-key script: %v", err)
		}
		gr.Passed = false
		gr.Skipped = false
	}
	return &gr
}
