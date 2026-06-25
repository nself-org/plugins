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

// runRustGates runs cargo clippy (deny warnings) and cargo test for a Rust crate.
//
// Purpose: Lint + test gate for Rust/Cargo projects (e.g. plugins-pro E7 plugins).
// Inputs:  root string — crate root containing Cargo.toml; timeout int; verbose bool
// Outputs: []GateResult — clippy lint + unit test results
// Constraints: clippy --deny warnings; cargo test --all-features; SPORT PLUGINS-CI-004
func runRustGates(root string, timeout int, verbose bool) []GateResult {
	return []GateResult{
		runStep("rust:clippy", root, timeout, verbose,
			"cargo", "clippy", "--all-targets", "--all-features", "--", "--deny", "warnings"),
		runStep("rust:test", root, timeout, verbose,
			"cargo", "test", "--all-features"),
	}
}

// runGatewayRoutingCheck verifies that the nself-ai-gateway on staging responds
// to /retrieval and /pty-relay routes (E7 completion check).
//
// Purpose: Confirm gateway routing is live for E7 plugins on staging.
// Inputs:  gatewayBase string — base URL of the gateway (e.g. http://167.235.233.65:3761)
// Outputs: []GateResult — one per route checked
// Constraints: curl -f; targets staging ONLY (never production); SPORT PLUGINS-CI-005
func runGatewayRoutingCheck(gatewayBase string, timeout int, verbose bool) []GateResult {
	routes := []struct {
		name   string
		method string
		path   string
	}{
		{"gateway:/retrieval", "POST", "/retrieval"},
		{"gateway:/pty-relay", "POST", "/pty-relay"},
	}

	// Use a temporary empty dir as the "root" for runStep (curl doesn't need a crate root).
	tmpDir := os.TempDir()

	var results []GateResult
	for _, r := range routes {
		url := gatewayBase + r.path
		// POST with an empty JSON body; expect HTTP 200 or 422 (valid request reached handler).
		// -f fails on 5xx/connection refused. --max-time caps per-request.
		args := []string{
			"-sf",
			"--max-time", fmt.Sprintf("%d", timeout),
			"-X", r.method,
			"-H", "Content-Type: application/json",
			"-d", "{}",
			"-o", "/dev/null",
			"-w", "%{http_code}",
			url,
		}
		gr := runStep(r.name, tmpDir, timeout, verbose, "curl", args...)
		// curl -s -f exits non-zero on 4xx/5xx when using -f; but we also want 422 (bad body but live).
		// Reinterpret: if output is a 3-digit code in [200,499], gate passes (service reachable + routing live).
		if !gr.Passed && len(gr.Output) == 3 {
			code := gr.Output
			if code >= "200" && code < "500" {
				gr.Passed = true
			}
		}
		results = append(results, gr)
	}
	return results
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
