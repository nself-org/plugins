package exec

// Purpose: read-only questions and one removal for the container runtime CLI, so
// isolation profiles probe and list through the one exec funnel.
// Inputs:  a runtime binary (docker or podman) and a fixed set of sub-commands.
// Outputs: the CLI's stdout, capped; errors carry E613.
// Constraints: no other sub-command is accepted, nothing here starts a job.

import (
	"context"
	"errors"
	"io"
	"io/fs"
	osexec "os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	maxRuntimeOutput = 1 << 20
	// queryTimeout bounds one read-only runtime question, so a wedged daemon cannot stall a caller.
	queryTimeout = 10 * time.Second
)

var (
	queryVerbs  = map[string]bool{"info": true, "version": true, "ps": true, "inspect": true}
	attemptName = regexp.MustCompile(`^nself-ci-[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

func runtimeFailed(err error) error {
	return &Error{Code: "E613", Message: "container runtime: " + err.Error(), Err: err}
}

// IsRuntimeMissing reports whether err means the runtime CLI is not installed.
func IsRuntimeMissing(err error) bool {
	return errors.Is(err, osexec.ErrNotFound) || errors.Is(err, fs.ErrNotExist)
}

// RuntimeQuery runs `<binary> <verb> ...` for info, version, ps or inspect and
// returns stdout (at most 1 MiB).
func RuntimeQuery(ctx context.Context, binary string, args ...string) ([]byte, error) {
	if !runtimeBinary.MatchString(binary) || len(args) == 0 || !queryVerbs[args[0]] {
		return nil, coded("E613", "container runtime query refused")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, binary, args...)
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
	if err := cmd.Wait(); err != nil {
		return out, runtimeFailed(err)
	}
	if readErr != nil {
		return out, runtimeFailed(readErr)
	}
	return out, nil
}

// RuntimeRemove force-removes the container of one attempt (`nself-ci-<id>`); a
// container that is already gone is not an error.
func RuntimeRemove(ctx context.Context, binary, name string) error {
	if !runtimeBinary.MatchString(binary) || !attemptName.MatchString(name) {
		return coded("E613", "container runtime removal refused")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, binary, "rm", "-f", name)
	cmd.Env = dockerHostEnv()
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "no such container") {
		return runtimeFailed(err)
	}
	return nil
}
