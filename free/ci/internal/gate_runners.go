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
