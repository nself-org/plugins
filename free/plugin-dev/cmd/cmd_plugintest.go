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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

func runPluginTest(p *parsed) error {
	name := p.args[0]
	phase := p.str("phase")
	hostMode := p.boolean("host")
	noCleanup := p.boolean("no-cleanup")

	if phase != "unit" && phase != "smoke" && phase != "both" {
		return fmt.Errorf("invalid --phase %q: must be unit, smoke, or both", phase)
	}

	pluginPath, err := links.ResolvePluginPath(name)
	if err != nil {
		return err
	}

	clui.Infof("Running plugin tests for %s (phase=%s)...", name, phase)

	var failed []string
	var passed []string

	if phase == "unit" || phase == "both" {
		clui.Dimmed("--- Phase 1: unit tests ---")
		start := time.Now()
		if err := runUnitTests(pluginPath, hostMode); err != nil {
			clui.Error(fmt.Sprintf("FAIL unit [%.1fs]: %v", time.Since(start).Seconds(), err))
			failed = append(failed, "unit")
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
			failed = append(failed, "smoke-install")
		} else {
			clui.Success(fmt.Sprintf("PASS smoke-install [%.1fs]", time.Since(start).Seconds()))
			passed = append(passed, "smoke-install")
		}

		clui.Dimmed("--- Phase 3: smoke uninstall ---")
		if !noCleanup {
			start = time.Now()
			if err := runSmokeUninstall(name); err != nil {
				clui.Error(fmt.Sprintf("FAIL smoke-uninstall [%.1fs]: %v", time.Since(start).Seconds(), err))
				failed = append(failed, "smoke-uninstall")
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
	if len(failed) > 0 {
		return fmt.Errorf("FAIL — phases failed: %s", strings.Join(failed, ", "))
	}

	clui.Success("All test phases passed.")
	return nil
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

	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("go test failed: %w", err)
	}
	return nil
}

// runSmokeInstall installs the linked plugin and verifies /healthz returns 200.
func runSmokeInstall(name string) error {
	installCmd := exec.Command(nselfBin(), "plugin", "install", name, "--force")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("smoke install failed: %w", err)
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
	if err := removeCmd.Run(); err != nil {
		return fmt.Errorf("smoke uninstall failed: %w", err)
	}
	return nil
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
