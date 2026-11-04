package exec

// Purpose: the narrow seam the agent's job-shim hand-off (D-1230) needs from the one package that may start
// processes: run one fixed argv with a given stdin and stdout, and return its capped stderr.
// Inputs:  the argv (program first), the environment, the working directory, stdin and stdout streams.
// Outputs: the first maxStreamStderr bytes of the child's stderr; the Run error, if any.
// Constraints: the program is chosen by the caller (the sudo chain of cmd/agent, replaced by a fake in tests),
// never ssh; the environment is exactly env, not the caller's; nothing is parsed or decided here.

import (
	"bytes"
	"context"
	"errors"
	"io"
	osexec "os/exec"
)

// maxStreamStderr bounds the stderr RunStreamed keeps: enough to name a failure, not to flood a log.
const maxStreamStderr = 4096

// RunStreamed runs argv with env in dir, stdin read from stdin (nil means none) and stdout written to stdout
// (nil discards it). It returns the first maxStreamStderr bytes of stderr, whether or not the run failed.
func RunStreamed(ctx context.Context, argv, env []string, dir string, stdin io.Reader, stdout io.Writer) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("run streamed: empty argv")
	}
	cmd := osexec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append([]string{}, env...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	if stdout == nil {
		stdout = io.Discard
	}
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &headWriter{w: &stderr, n: maxStreamStderr}
	err := cmd.Run()
	return stderr.String(), err
}

// headWriter keeps the first n bytes and drops the rest, always reporting the full write.
type headWriter struct {
	w io.Writer
	n int
}

func (l *headWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
