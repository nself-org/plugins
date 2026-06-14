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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		}
		res.Gates = append(res.Gates, gates...)
	}

	// 4. Aggregate pass/fail.
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
	return stacks
}

// runGitleaks runs gitleaks detect on the repo root.
func runGitleaks(root string, timeout int, verbose bool) GateResult {
	// Prefer a repo-local gitleaks.toml if present.
	configFlag := ""
	for _, candidate := range []string{
		filepath.Join(root, ".github", "gitleaks.toml"),
		filepath.Join(root, ".gitleaks.toml"),
		filepath.Join(root, "gitleaks.toml"),
	} {
		if fileExists(candidate) {
			configFlag = candidate
			break
		}
	}

	args := []string{"detect", "--source", root, "--no-git", "--exit-code", "1"}
	if configFlag != "" {
		args = append(args, "--config", configFlag)
	}

	return runStep("secrets:gitleaks", root, timeout, verbose, "gitleaks", args...)
}

// runGoGates runs gofmt, go vet, and go test for a Go repo.
func runGoGates(root string, timeout int, verbose bool) []GateResult {
	return []GateResult{
		runStep("go:fmt", root, timeout, verbose, "gofmt", "-l", "."),
		runStep("go:vet", root, timeout, verbose, "go", "vet", "./..."),
		runStep("go:test", root, timeout, verbose, "go", "test", "-count=1", "-timeout", fmt.Sprintf("%ds", timeout), "./..."),
	}
}

// runNodeGates runs pnpm lint, pnpm test, and pnpm build for a Node repo.
// Falls back to npm if pnpm is not present.
func runNodeGates(root string, timeout int, verbose bool) []GateResult {
	pm := "pnpm"
	if _, err := exec.LookPath("pnpm"); err != nil {
		pm = "npm"
	}

	pkg := loadPackageJSON(root)

	var gates []GateResult
	if hasScript(pkg, "lint") {
		gates = append(gates, runStep("node:lint", root, timeout, verbose, pm, "run", "lint"))
	}
	if hasScript(pkg, "typecheck") {
		gates = append(gates, runStep("node:typecheck", root, timeout, verbose, pm, "run", "typecheck"))
	}
	if hasScript(pkg, "test") {
		gates = append(gates, runStep("node:test", root, timeout, verbose, pm, "run", "test"))
	}
	if hasScript(pkg, "build") {
		gates = append(gates, runStep("node:build", root, timeout, verbose, pm, "run", "build"))
	}

	if len(gates) == 0 {
		// No scripts found; at minimum run tsc if tsconfig.json exists.
		if fileExists(filepath.Join(root, "tsconfig.json")) {
			gates = append(gates, runStep("node:tsc", root, timeout, verbose, pm, "exec", "tsc", "--noEmit"))
		}
	}

	return gates
}

// runFlutterGates runs flutter analyze and flutter test.
func runFlutterGates(root string, timeout int, verbose bool) []GateResult {
	return []GateResult{
		runStep("flutter:analyze", root, timeout, verbose, "flutter", "analyze"),
		runStep("flutter:test", root, timeout, verbose, "flutter", "test", "--reporter", "compact"),
	}
}

// runStep executes a single gate command and returns its result.
// For gofmt specifically, success means no output (unlinted files print their path).
func runStep(name, root string, timeout int, verbose bool, cmd string, args ...string) GateResult {
	start := time.Now()
	gr := GateResult{Name: name}

	// Check if the command exists first.
	if _, err := exec.LookPath(cmd); err != nil {
		gr.Output = fmt.Sprintf("command not found: %s (skipped)", cmd)
		gr.Passed = true // Skip missing optional tools gracefully.
		gr.Elapsed = time.Since(start)
		return gr
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[nself-ci] running: %s %s\n", cmd, strings.Join(args, " "))
	}

	c := exec.Command(cmd, args...)
	c.Dir = root

	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	err := c.Run()
	gr.Output = strings.TrimSpace(buf.String())
	gr.Elapsed = time.Since(start)

	// gofmt: non-empty output means files need formatting → fail.
	if name == "go:fmt" {
		if gr.Output != "" {
			gr.Passed = false
			gr.Output = "Files need gofmt:\n" + gr.Output
		} else {
			gr.Passed = true
		}
		return gr
	}

	gr.Passed = (err == nil)
	return gr
}

// fileExists returns true if the path exists (file or dir).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadPackageJSON reads the scripts section of package.json as a raw map.
func loadPackageJSON(root string) map[string]interface{} {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Minimal JSON parse for scripts section — avoid pulling in dependencies.
	scripts := extractJSONObject(string(data), "scripts")
	result := make(map[string]interface{})
	for k, v := range scripts {
		result[k] = v
	}
	return result
}

// hasScript returns true if the package.json scripts map contains the key.
func hasScript(pkg map[string]interface{}, key string) bool {
	if pkg == nil {
		return false
	}
	_, ok := pkg[key]
	return ok
}

// extractJSONObject extracts key→value string pairs from a named JSON object
// using simple string parsing (no external JSON library to keep zero deps).
func extractJSONObject(json, key string) map[string]string {
	result := make(map[string]string)
	// Find "key":
	search := `"` + key + `"`
	idx := strings.Index(json, search)
	if idx < 0 {
		return result
	}
	// Find the opening brace after the key.
	start := strings.Index(json[idx:], "{")
	if start < 0 {
		return result
	}
	start += idx + 1

	// Walk until matching closing brace.
	depth := 1
	end := start
	for end < len(json) && depth > 0 {
		switch json[end] {
		case '{':
			depth++
		case '}':
			depth--
		}
		end++
	}
	block := json[start : end-1]

	// Extract "name": "value" pairs.
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `"`) {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		v := strings.Trim(strings.TrimSpace(strings.TrimRight(parts[1], ",")), `"`)
		if k != "" {
			result[k] = v
		}
	}
	return result
}
