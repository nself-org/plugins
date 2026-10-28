//go:build !windows

package exec

// The toolchain image owns PATH and GOTOOLCHAIN: a host value forwarded into the
// container env file would hide the image's tools or let a job fetch an unpinned Go
// (D-1182, D-1183).

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestContainerEnvDropsImageOwnedNames(t *testing.T) {
	for name, tc := range map[string]struct{ in, want []string }{
		"path":            {[]string{"PATH=/usr/bin", "LANG=C"}, []string{"LANG=C"}},
		"toolchain":       {[]string{"GOTOOLCHAIN=auto", "GOFLAGS=-mod=mod"}, []string{"GOFLAGS=-mod=mod"}},
		"lowercase":       {[]string{"path=/x", "Gotoolchain=auto"}, []string{}},
		"value-has-eq":    {[]string{"FOO=PATH=/x"}, []string{"FOO=PATH=/x"}},
		"prefixed":        {[]string{"PATHEXT=.exe", "MYPATH=/x", "GOTOOLCHAIN_X=1"}, []string{"PATHEXT=.exe", "MYPATH=/x", "GOTOOLCHAIN_X=1"}},
		"toolchain-homes": {[]string{"GOROOT=/opt/go", "GOOS=darwin", "GOARCH=arm64", "CARGO_HOME=/h/.cargo", "RUSTUP_HOME=/h/.rustup", "PNPM_HOME=/h/p", "NVM_DIR=/h/.nvm", "GOFLAGS=-mod=mod"}, []string{"GOFLAGS=-mod=mod"}},
		"empty":           {nil, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := containerEnv(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("containerEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// runPlannedEnv runs a planned container job under the allowlist policy with the host
// values set, and returns the env file the runtime read from stdin.
func runPlannedEnv(t *testing.T, jobEnv map[string]string) string {
	t.Helper()
	_, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	t.Setenv("GOTOOLCHAIN", "auto")
	t.Setenv("LANG", "C")
	spec := plannedSpec(t, "plan-envown", hardPlanner{})
	spec.Job.Env = jobEnv
	if res := (&Executor{Isolation: BaselineIsolator{}, Env: AllowlistEnv{}}).Run(context.Background(), spec); res.Err != nil {
		t.Fatal(res.Err)
	}
	return stdin()
}

func hasEnvLine(file, line string) bool { return slices.Contains(strings.Split(file, "\n"), line) }

func TestPlannedContainerKeepsImagePathAndToolchain(t *testing.T) {
	file := runPlannedEnv(t, nil)
	for _, l := range strings.Split(file, "\n") {
		if strings.HasPrefix(l, "PATH=") || strings.HasPrefix(l, "GOTOOLCHAIN=") {
			t.Fatalf("host %q reached the container env file:\n%s", l, file)
		}
	}
	if !hasEnvLine(file, "LANG=C") {
		t.Fatalf("other allowlisted variables must still pass:\n%s", file)
	}
}

func TestPlannedContainerJobDeclaredEnvStillApplies(t *testing.T) {
	file := runPlannedEnv(t, map[string]string{"PATH": "/opt/job/bin:/usr/bin"})
	if !hasEnvLine(file, "PATH=/opt/job/bin:/usr/bin") || strings.Contains(file, "GOTOOLCHAIN=") {
		t.Fatalf("env file:\n%s", file)
	}
}

func TestBaselineContainerKeepsImagePathAndToolchain(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "job.env")
	fakeDocker(t, "for a in \"$@\"; do if [ \"$prev\" = --env-file ]; then cp \"$a\" "+filePath+"; fi; prev=\"$a\"; done\nexit 0\n")
	t.Setenv("GOTOOLCHAIN", "auto")
	r := (&Executor{Isolation: BaselineIsolator{}, Env: AllowlistEnv{}}).Run(context.Background(), JobSpec{AttemptID: "envown", CoordinatorID: "coord", Home: t.TempDir(),
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"true"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "PATH=") || strings.HasPrefix(l, "GOTOOLCHAIN=") {
			t.Fatalf("host %q reached the baseline container env file:\n%s", l, b)
		}
	}
}

// D-1185: a host with podman and no docker runs the baseline container with podman.
func TestBaselineContainerUsesPodmanWhenDockerIsAbsent(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "podman-ran")
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/sh\n: > "+marker+"\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	r := (&Executor{Isolation: BaselineIsolator{}, Env: AllowlistEnv{}}).Run(context.Background(), JobSpec{AttemptID: "podman", CoordinatorID: "coord", Home: t.TempDir(),
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Command: []string{"true"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("podman was not run: %v", err)
	}
}
