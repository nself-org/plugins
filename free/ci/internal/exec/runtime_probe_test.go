//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const probeDigest = "sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"

// fakeProbeRuntime puts a `docker` script on PATH that logs its argv and prints stdout; code is its exit status.
func fakeProbeRuntime(t *testing.T, stdout string, code int) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s|' \"$@\" >> '" + logFile + "'\necho >> '" + logFile + "'\n[ \"$1\" = run ] && printf '%s' '" + stdout + "'\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func TestRuntimeProbeRunRefusals(t *testing.T) {
	good := "docker.io/library/busybox@" + probeDigest
	for name, tc := range map[string]struct{ binary, runtime, image string }{
		"unknown binary":   {"nerdctl", "runsc", good},
		"other runtime":    {"docker", "kata", good},
		"empty runtime":    {"docker", "", good},
		"image by tag":     {"docker", "runsc", "docker.io/library/busybox:1.37"},
		"image traversal":  {"docker", "runsc", "docker.io/library/../x@" + probeDigest},
		"image with space": {"docker", "runsc", "busybox @" + probeDigest},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := RuntimeProbeRun(context.Background(), tc.binary, tc.runtime, tc.image); err == nil || !strings.Contains(err.Error(), "E613") {
				t.Fatalf("want E613, got %v", err)
			}
		})
	}
}

func TestRuntimeProbeRunBuildsAHardenedFixedCommand(t *testing.T) {
	logFile := fakeProbeRuntime(t, "[    0.000000] Starting gVisor...", 0)
	image := "docker.io/library/busybox@" + probeDigest
	out, err := RuntimeProbeRun(context.Background(), "docker", "runsc", image)
	if err != nil || !strings.Contains(string(out), "Starting gVisor") {
		t.Fatalf("out %q err %v", out, err)
	}
	logged, _ := os.ReadFile(logFile)
	argv := strings.TrimSpace(string(logged))
	for _, want := range []string{"run|--rm|--name|nself-ci-probe-", "|--runtime=runsc|", "|--network=none|", "|--cap-drop=ALL|",
		"|--security-opt|no-new-privileges|", "|--read-only|", "|--pids-limit|64|", "|--memory=64m|", "|--user|65534:65534|", "|" + image + "|dmesg|"} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv lacks %q: %s", want, argv)
		}
	}
	for _, bad := range []string{"--privileged", "--network=host", "|-v|", "--volume", "--mount", ".sock"} {
		if strings.Contains(argv, bad) {
			t.Errorf("argv carries %q: %s", bad, argv)
		}
	}
}

func TestRuntimeProbeRunRemovesTheContainerOnFailure(t *testing.T) {
	logFile := fakeProbeRuntime(t, "", 1)
	if _, err := RuntimeProbeRun(context.Background(), "docker", "runsc", "docker.io/library/busybox@"+probeDigest); err == nil || !strings.Contains(err.Error(), "E613") {
		t.Fatalf("want E613, got %v", err)
	}
	logged, _ := os.ReadFile(logFile)
	if !strings.Contains(string(logged), "rm|-f|nself-ci-probe-") {
		t.Fatalf("no cleanup after a failed probe: %s", logged)
	}
}

func TestRuntimeToolVersion(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "runsc")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho 'runsc version release-20261012.0'\necho 'spec: 1.1.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := RuntimeToolVersion(context.Background(), tool)
	if err != nil || got != "runsc version release-20261012.0" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, path := range []string{"", "runsc", "relative/runsc", filepath.Join(dir, "other"), "/bin/sh"} {
		if _, err := RuntimeToolVersion(context.Background(), path); err == nil || !strings.Contains(err.Error(), "E613") {
			t.Errorf("%s: want E613, got %v", path, err)
		}
	}
}

func TestCheckSandboxRuntime(t *testing.T) {
	image := "docker.io/library/busybox@" + probeDigest
	with := ContainerCommand{Binary: "docker", Args: []string{"run", "--runtime=runsc", "--rm", image, "sh"}}
	without := ContainerCommand{Binary: "docker", Args: []string{"run", "--rm", image, "sh"}}
	inJob := ContainerCommand{Binary: "docker", Args: []string{"run", "--rm", image, "echo", "--runtime=runsc"}} // a job word, after the image
	if err := checkSandboxRuntime("sandboxed-container", with, image); err != nil {
		t.Fatalf("runsc command refused: %v", err)
	}
	for name, cmd := range map[string]ContainerCommand{"no runtime": without, "runtime only in the job words": inJob} {
		if err := checkSandboxRuntime("sandboxed-container", cmd, image); err == nil || !strings.Contains(err.Error(), "E613") {
			t.Errorf("%s: want E613, got %v", name, err)
		}
	}
	if err := checkSandboxRuntime("container", without, image); err != nil {
		t.Errorf("the container level needs no runtime: %v", err)
	}
}
