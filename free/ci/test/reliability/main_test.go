//go:build integration

package reliability

// Purpose: the lost-runner reliability suite (P7-NODE-11): fault injection against real nself-ci-agent processes in
//          Docker nodes (simharness) and a coordinator made of the real store, PKI, agent listener, agent
//          transport driver, lease engine and run loop. One test per fault class, each named TestFault*.
// Inputs:  INTEGRATION=1 NSELF_V15=1, a Linux Docker Engine, go for the linux agent build.
// Outputs: pass or fail. A missing prerequisite is a t.Fatal, never a skip (the verify leg counts the tests).
// Constraints: every value is synthetic; nothing leaves 127.0.0.1 and the Docker network; no bare sleeps (event
//          waits with a deadline, or an injected clock); containers and the network are removed by simharness.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	agentOnce sync.Once
	agentPath string
	agentErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

// needIntegration fails (never skips) when the suite is not run the way the verify leg runs it.
func needIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("run with INTEGRATION=1 -tags integration")
	}
	t.Setenv("NSELF_V15", "1")
}

// linuxAgent builds nself-ci-agent once per test binary for the architecture of the Docker engine.
func linuxAgent(t *testing.T) string {
	t.Helper()
	agentOnce.Do(func() {
		arch := dockerArch()
		var err error
		if buildDir, err = os.MkdirTemp("", "reliability-agent-"); err != nil {
			agentErr = err
			return
		}
		agentPath = filepath.Join(buildDir, "nself-ci-agent")
		cmd := exec.Command("go", "build", "-mod=vendor", "-o", agentPath, "./cmd/agent")
		cmd.Dir = "../.."
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		if out, berr := cmd.CombinedOutput(); berr != nil {
			agentErr = &buildError{err: berr, out: string(out)}
		}
	})
	if agentErr != nil {
		t.Fatalf("build the linux agent: %v", agentErr)
	}
	return agentPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// dockerArch is the architecture the Docker engine runs containers on ("amd64" when it cannot be read).
func dockerArch() string {
	if out, err := exec.Command("docker", "info", "--format", "{{.Architecture}}").Output(); err == nil {
		if s := string(out); strings.Contains(s, "aarch64") || strings.Contains(s, "arm64") {
			return "arm64"
		}
	}
	return "amd64"
}
