package main

// Purpose: `nself-ci-agent cred-wrapper <kind> -- <command> [args]` hands a
// runner credential to a runner process without ever putting it in argv, a
// container `-e` flag or a file (HOST review R3).
// Inputs:  the credential as one line on stdin; <kind> is github-jitconfig or
// gitlab-runner-token.
// Outputs: none; the process becomes <command> (execve), so the credential
// lives only in that process's environment.
// Constraints: the agent's own environment is not passed on (only a fixed
// allowlist plus the credential variable); the line is read with a bound and
// zeroed once the environment is built; there is no output on success.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var credEnv = map[string]string{
	"github-jitconfig":    "ACTIONS_RUNNER_INPUT_JITCONFIG",
	"gitlab-runner-token": "CI_SERVER_TOKEN",
}

// execve replaces the process; tests swap it.
var execve = syscall.Exec

func init() {
	must(RegisterSubcommand("cred-wrapper", "start a runner command with a credential read from stdin", runCredWrapper))
}

func runCredWrapper(ctx context.Context, args []string, sio IO) int {
	if len(args) < 3 || args[1] != "--" || credEnv[args[0]] == "" {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent cred-wrapper <github-jitconfig|gitlab-runner-token> -- <command> [args...]")
		return 2
	}
	line, err := readCredential(sio.Stdin)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	defer zeroBytes(line)
	path, err := lookCommand(args[2])
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	env, err := ciexec.AllowlistEnv{}.Environment(ctx, model.Job{})
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	env = append(env, credEnv[args[0]]+"="+string(line))
	err = execve(path, args[2:], env)
	fmt.Fprintln(sio.Stderr, "error: exec:", err)
	return 1
}

// readCredential reads one line (at most 64 KiB) from r.
func readCredential(r io.Reader) ([]byte, error) {
	br := bufio.NewReaderSize(r, 4096)
	var line []byte
	for len(line) <= 64<<10 {
		frag, err := br.ReadSlice('\n')
		line = append(line, frag...)
		if err == nil {
			line = []byte(strings.TrimRight(string(line), "\r\n"))
			if len(line) == 0 {
				return nil, errors.New("empty credential on stdin")
			}
			return line, nil
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return []byte(strings.TrimRight(string(line), "\r\n")), nil
			}
			return nil, errors.New("no credential on stdin")
		}
	}
	return nil, errors.New("credential line too long")
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// lookCommand resolves a bare name through the absolute entries of PATH
// (execve needs a path, and os/exec is internal/exec's alone).
func lookCommand(name string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return name, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: command not found", name)
}
