package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (r *Result) Summary() string {
	if r.Passed {
		stacks := strings.Join(r.Stack, "+")
		return fmt.Sprintf("All gates passed (%s) in %s", stacks, r.Elapsed.Round(time.Second))
	}
	for _, g := range r.Gates {
		if !g.Passed {
			return fmt.Sprintf("Gate failed: %s", g.Name)
		}
	}
	return "Gate failed"
}

// Run executes the CI gate suite for the repo at cfg.RepoRoot.
// Size-cap exception: config loader — 71L of env-var reads with validation; single cohesive unit, splitting fragments the config contract.
func Run(cfg Config) (*Result, error) {
	root := cfg.RepoRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot determine working directory: %w", err)
		}
	}

	// Resolve to absolute path.
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path %q: %w", cfg.RepoRoot, err)
	}

	res := &Result{RepoRoot: root}
	start := time.Now()

	// 1. Detect stacks present.
	res.Stack = detectStacks(root)
	if len(res.Stack) == 0 {
		return nil, fmt.Errorf("no supported stack detected in %s (need go.mod, package.json, pubspec.yaml)", root)
	}

	timeout := cfg.StepTimeout
	if timeout <= 0 {
		timeout = 120
	}

	// 2. Run gitleaks scan first (fast, fail-fast on secrets).
	if !cfg.SkipGitleaks {
		gr := runGitleaks(root, timeout, cfg.Verbose)
		res.Gates = append(res.Gates, gr)
	}

	// 3. Run stack-specific gates.
	for _, stack := range res.Stack {
		var gates []GateResult
		switch stack {
		case "go":
			gates = runGoGates(root, timeout, cfg.Verbose)
		case "node":
			gates = runNodeGates(root, timeout, cfg.Verbose)
		case "flutter":
			gates = runFlutterGates(root, timeout, cfg.Verbose)
		case "rust":
			gates = runRustGates(root, timeout, cfg.Verbose)
		}
		res.Gates = append(res.Gates, gates...)
	}

	// 4. Gateway routing check (E7 completion gate — runs after all stack gates).
	// Targets staging only; never production. SPORT: PLUGINS-CI-005
	if cfg.GatewayBase != "" {
		gwGates := runGatewayRoutingCheck(cfg.GatewayBase, timeout, cfg.Verbose)
		res.Gates = append(res.Gates, gwGates...)
	}

	// 5. Aggregate pass/fail.
	res.Passed = true
	for _, g := range res.Gates {
		if !g.Passed {
			res.Passed = false
			break
		}
	}

	res.Elapsed = time.Since(start)
	return res, nil
}

// detectStacks returns which stacks are present in the repo root.
// Rust is detected by Cargo.toml presence.
func detectStacks(root string) []string {
	var stacks []string
	if fileExists(filepath.Join(root, "go.mod")) {
		stacks = append(stacks, "go")
	}
	if fileExists(filepath.Join(root, "package.json")) {
		stacks = append(stacks, "node")
	}
	if fileExists(filepath.Join(root, "pubspec.yaml")) {
		stacks = append(stacks, "flutter")
	}
	if fileExists(filepath.Join(root, "Cargo.toml")) {
		stacks = append(stacks, "rust")
	}
	return stacks
}

// runGitleaks runs gitleaks detect on the repo root.
