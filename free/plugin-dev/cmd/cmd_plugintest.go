package main

// Purpose: `test <name>`: run a linked plugin's unit tests and a smoke
// install/uninstall. Port of core cmd/commands/plugin_test_cmd.go.
// Inputs:  a plugin name; --phase unit|smoke|both, --host, --no-cleanup.
// Outputs: PASS/FAIL lines per phase and a summary; runs `go test` (or docker),
// `nself plugin install <name> --force`, `nself plugin remove <name>` and curl.
// Constraints: nothing is installed unless a smoke phase runs; the nself CLI is
// found as "nself" on PATH (core uses its own executable, which a plugin cannot).
// The file name avoids the _test suffix, which Go reserves for test files.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

// testData is the --json data of `test` when every phase run passed.
type testData struct {
	Name    string   `json:"name"`
	Phase   string   `json:"phase"`
	Passed  []string `json:"passed"`
	Cleanup bool     `json:"cleanup"`
}

func runPluginTest(p *parsed) (any, error) {
	name := p.args[0]
	phase := p.str("phase")
	hostMode := p.boolean("host")
	noCleanup := p.boolean("no-cleanup")

	if phase != "unit" && phase != "smoke" && phase != "both" {
		return nil, usageErr(fmt.Errorf("invalid --phase %q: must be unit, smoke, or both", phase),
			"invalid --phase: must be unit, smoke, or both")
	}

	pluginPath, err := links.ResolvePluginPath(name)
	if err != nil {
		return nil, pluginNotFoundErr(err)
	}

	clui.Infof("Running plugin tests for %s (phase=%s)...", name, phase)

	var failed []string
	var passed []string
	var infra error // the first phase error that was a launch failure, not a test failure
	noteFail := func(phase string, err error) {
		failed = append(failed, phase)
		var le *launchError
		if infra == nil && errors.As(err, &le) {
			infra = err
		}
	}

	if phase == "unit" || phase == "both" {
		clui.Dimmed("--- Phase 1: unit tests ---")
		start := time.Now()
		if err := runUnitTests(pluginPath, hostMode); err != nil {
			clui.Error(fmt.Sprintf("FAIL unit [%.1fs]: %v", time.Since(start).Seconds(), err))
			noteFail("unit", err)
		} else {
			clui.Success(fmt.Sprintf("PASS unit [%.1fs]", time.Since(start).Seconds()))
			passed = append(passed, "unit")
		}
	}

	if phase == "smoke" || phase == "both" {
		clui.Dimmed("--- Phase 2: smoke install ---")
		start := time.Now()
		if err := runSmokeInstall(name); err != nil {
			clui.Error(fmt.Sprintf("FAIL smoke-install [%.1fs]: %v", time.Since(start).Seconds(), err))
			noteFail("smoke-install", err)
		} else {
			clui.Success(fmt.Sprintf("PASS smoke-install [%.1fs]", time.Since(start).Seconds()))
			passed = append(passed, "smoke-install")
		}

		clui.Dimmed("--- Phase 3: smoke uninstall ---")
		if !noCleanup {
			start = time.Now()
			if err := runSmokeUninstall(name); err != nil {
				clui.Error(fmt.Sprintf("FAIL smoke-uninstall [%.1fs]: %v", time.Since(start).Seconds(), err))
				noteFail("smoke-uninstall", err)
			} else {
				clui.Success(fmt.Sprintf("PASS smoke-uninstall [%.1fs]", time.Since(start).Seconds()))
				passed = append(passed, "smoke-uninstall")
			}
		} else {
			clui.Dimmed("Skipping uninstall (--no-cleanup).")
		}
	}

	clui.Dimmed("---")
	if len(passed) > 0 {
		clui.Dimmedf("Passed: %s", strings.Join(passed, ", "))
	}
	if infra != nil {
		return nil, stepErr(infra, "a test tool could not be launched in: "+strings.Join(failed, ", "),
			"install go (and docker or the nself CLI for the smoke phases) and put them on PATH; the tool output is on stderr")
	}
	if len(failed) > 0 {
		return nil, coded(fmt.Errorf("FAIL — phases failed: %s", strings.Join(failed, ", ")),
			CodeTestsFailed, output.ClassUser, "plugin tests failed in: "+strings.Join(failed, ", "),
			"the test output is on stderr")
	}

	clui.Success("All test phases passed.")
	return testData{Name: name, Phase: phase, Passed: append([]string{}, passed...), Cleanup: !noCleanup && phase != "unit"}, nil
}

// runUnitTests runs go test ./... in the plugin source directory (in a golang
// container unless --host or Docker is unavailable).
func runUnitTests(pluginPath string, hostMode bool) error {
	var testCmd *exec.Cmd
	if hostMode {
		testCmd = exec.Command("go", "test", "./...", "-v", "-count=1")
		testCmd.Dir = pluginPath
	} else {
		if isDockerAvailable() {
			testCmd = exec.Command("docker", "run", "--rm",
				"-v", pluginPath+":/workspace",
				"-w", "/workspace",
				"golang:1.22-alpine",
				"sh", "-c", "go test ./... -v -count=1",
			)
		} else {
			clui.Warn("Docker not available; running unit tests on host.")
			testCmd = exec.Command("go", "test", "./...", "-v", "-count=1")
			testCmd.Dir = pluginPath
		}
	}

	testCmd.Stdout = os.Stdout
	testCmd.Stderr = os.Stderr

	return childErr(testCmd.Run(), "go test failed")
}

// launchError marks a child that could not be started (tool missing, not
// executable), as opposed to one that ran and exited non-zero. The first is an
// infrastructure failure (E776), the second a test failure (E773).
type launchError struct{ err error }

func (e *launchError) Error() string { return e.err.Error() }
func (e *launchError) Unwrap() error { return e.err }

// childErr wraps the error of cmd.Run with what: a launch failure becomes a
// *launchError, an *exec.ExitError stays a plain wrapped error.
func childErr(err error, what string) error {
	if err == nil {
		return nil
	}
	err = fmt.Errorf("%s: %w", what, err)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return err
	}
	return &launchError{err: err}
}

// runSmokeInstall installs the linked plugin and verifies /healthz returns 200.
func runSmokeInstall(name string) error {
	installCmd := exec.Command(nselfBin(), "plugin", "install", name, "--force")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := childErr(installCmd.Run(), "smoke install failed"); err != nil {
		return err
	}

	healthURL := resolvePluginHealthURL(name)
	if err := waitForHealth(healthURL, 5, 6*time.Second); err != nil {
		return fmt.Errorf("health check failed after install: %w", err)
	}
	return nil
}

// runSmokeUninstall removes the plugin and verifies clean state.
func runSmokeUninstall(name string) error {
	removeCmd := exec.Command(nselfBin(), "plugin", "remove", name)
	removeCmd.Stdout = os.Stdout
	removeCmd.Stderr = os.Stderr
	return childErr(removeCmd.Run(), "smoke uninstall failed")
}

// resolvePluginHealthURL builds the expected health check URL for a plugin.
func resolvePluginHealthURL(name string) string {
	base := os.Getenv("NSELF_LOCAL_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	return fmt.Sprintf("%s/healthz", base)
}

// waitForHealth polls the URL until it returns 200 or retries are exhausted.
func waitForHealth(url string, retries int, interval time.Duration) error {
	for i := 0; i < retries; i++ {
		resp, err := httpGetWithTimeout(url, 5*time.Second)
		if err == nil && resp == 200 {
			return nil
		}
		if i < retries-1 {
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("health check at %s did not return 200 after %d retries", url, retries)
}

// httpGetWithTimeout performs a GET with curl and returns the status code.
func httpGetWithTimeout(url string, timeout time.Duration) (int, error) {
	curlCmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"--max-time", fmt.Sprintf("%.0f", timeout.Seconds()), url)
	out, err := curlCmd.Output()
	if err != nil {
		return 0, err
	}
	var code int
	if _, err := fmt.Sscan(string(out), &code); err != nil {
		return 0, err
	}
	return code, nil
}

// isDockerAvailable returns true if the docker CLI is on PATH and responds.
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = os.Stderr // discard
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}
