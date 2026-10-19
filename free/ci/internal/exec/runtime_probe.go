package exec

// Purpose: the two starts the sandboxed-container profile needs and RuntimeQuery cannot do:
// one short-lived hardened probe container under a named OCI runtime, and `runsc --version`.
// Inputs:  a runtime binary (docker or podman), the runtime name (runsc only), an image pinned
// by digest; or the absolute runsc path the daemon is configured with.
// Outputs: the probe's stdout (capped) or the first line of the version output.
// Constraints: argv is built here from fixed words, never from the caller; the probe command is
// always `dmesg`, with no network, no capabilities, a read-only root and small limits; the
// container is removed on every exit path. Nothing here starts a job.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	osexec "os/exec"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	// probeTimeout bounds one probe start (an image pull on a cold host is the slow part).
	probeTimeout = 90 * time.Second
	// sandboxRuntime is the one OCI runtime a probe may name; the planned-container allowlist agrees.
	sandboxRuntime = "runsc"
	maxVersionLine = 256
)

var probeImageRef = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*@sha256:[0-9a-f]{64}$`)

// RuntimeProbeRun starts `<binary> run --runtime=runsc ... <image> dmesg` and returns its stdout.
// The image must be pinned by digest.
func RuntimeProbeRun(ctx context.Context, binary, runtime, image string) ([]byte, error) {
	if !runtimeBinary.MatchString(binary) || runtime != sandboxRuntime || !probeImageRef.MatchString(image) || strings.Contains(image, "..") {
		return nil, coded("E613", "container runtime probe refused")
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, runtimeFailed(err)
	}
	name := "nself-ci-probe-" + hex.EncodeToString(suffix)
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, binary, "run", "--rm", "--name", name, "--label", "nself.ci.probe=1",
		"--runtime="+runtime, "--network=none", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only",
		"--pids-limit", "64", "--memory=64m", "--memory-swap=64m", "--cpus=0.5", "--user", "65534:65534", image, "dmesg")
	cmd.Env = dockerHostEnv()
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, runtimeFailed(err)
	}
	if err := cmd.Start(); err != nil {
		return nil, runtimeFailed(err)
	}
	out, readErr := io.ReadAll(io.LimitReader(pipe, maxRuntimeOutput))
	_, _ = io.Copy(io.Discard, pipe)
	waitErr := cmd.Wait()
	if ctx.Err() != nil || waitErr != nil {
		// a killed CLI can leave the container behind: remove it, best effort, on a fresh time box
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), queryTimeout)
		_ = RuntimeRemove(rctx, binary, name)
		rcancel()
	}
	if waitErr != nil {
		return out, runtimeFailed(waitErr)
	}
	if readErr != nil {
		return out, runtimeFailed(readErr)
	}
	return out, nil
}

// RuntimeToolVersion runs `<path> --version` for runsc and returns the first output line.
// path must be absolute with base name runsc: it is the binary the daemon is configured with, so
// a PATH lookup that finds a different runsc is never asked.
func RuntimeToolVersion(ctx context.Context, path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Base(path) != sandboxRuntime {
		return "", coded("E613", "runtime tool version refused")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, path, "--version")
	cmd.Env = dockerHostEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", runtimeFailed(err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if len(line) > maxVersionLine {
		line = line[:maxVersionLine]
	}
	return strings.TrimSpace(line), nil
}

// checkSandboxRuntime refuses a sandboxed-container job whose planned command does not run under
// runsc, so the recorded level can never be stronger than the command that ran (a later planner
// change that drops the flag fails here instead of running as a plain container). Only the flag
// region (before the image) is read.
func checkSandboxRuntime(level model.Isolation, cmd ContainerCommand, image string) error {
	if level != "sandboxed-container" {
		return nil
	}
	for i := 1; i < len(cmd.Args) && cmd.Args[i] != image; i++ {
		if cmd.Args[i] == "--runtime="+sandboxRuntime {
			return nil
		}
	}
	return coded("E613", "sandboxed-container job planned without --runtime="+sandboxRuntime)
}
