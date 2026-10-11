// Purpose: the seam that runs a command INSIDE the postgres container, and its
// two implementations (kubectl exec for a cluster, docker exec for the integration
// test).
//
// Inputs: an argv to run in the container, stdin and the output writers.
//
// Outputs: Run waits for the command; Start returns a Session with a stdin pipe
// and a stdout reader for an interactive psql.
//
// Constraints: the environment of the local process is os.Environ() minus every
// NSELF_* variable. No value (password, token) ever travels on a local argv: the
// commands run in the pod read their settings from the pod's own environment
// (scripts.go). Tests use a fake Exec.
package ops

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Exec runs commands inside the postgres container.
type Exec interface {
	// Run runs argv to completion. stdin may be nil.
	Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error
	// Start starts argv with a stdin pipe and a stdout pipe. stderr may be nil.
	Start(ctx context.Context, argv []string, stderr io.Writer) (Session, error)
}

// Session is a running command with pipes.
type Session interface {
	// Stdin is the command's standard input; Close ends it.
	Stdin() io.WriteCloser
	// Stdout is the command's standard output.
	Stdout() io.Reader
	// Wait waits for the command to end and releases its resources.
	Wait() error
	// Kill stops the command (no-op once it has ended).
	Kill()
}

// cmdExec runs prefix+argv as a local process.
type cmdExec struct {
	bin    string
	prefix []string
	env    []string
}

func (c *cmdExec) command(ctx context.Context, argv []string) *exec.Cmd {
	full := append(append([]string{}, c.prefix...), argv...)
	cmd := exec.CommandContext(ctx, c.bin, full...)
	cmd.Env = c.env
	return cmd
}

func (c *cmdExec) Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := c.command(ctx, argv)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

func (c *cmdExec) Start(ctx context.Context, argv []string, stderr io.Writer) (Session, error) {
	cmd := c.command(ctx, argv)
	cmd.Stderr = stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procSession{cmd: cmd, in: in, out: out}, nil
}

type procSession struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.Reader
}

func (s *procSession) Stdin() io.WriteCloser { return s.in }
func (s *procSession) Stdout() io.Reader     { return s.out }
func (s *procSession) Wait() error           { return s.cmd.Wait() }
func (s *procSession) Kill() {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// cleanEnv returns parent without NSELF_* variables.
func cleanEnv(parent []string) []string {
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		if !strings.HasPrefix(kv, "NSELF_") {
			out = append(out, kv)
		}
	}
	return out
}

// NewDocker runs commands with `docker exec -i <container>`. The integration test
// uses it against a throwaway postgres container.
func NewDocker(container string) Exec {
	return &cmdExec{bin: "docker", prefix: []string{"exec", "-i", container}, env: cleanEnv(os.Environ())}
}

// boundedBuffer keeps the last max bytes written to it.
type boundedBuffer struct {
	max int
	buf []byte
}

func newBounded(max int) *boundedBuffer { return &boundedBuffer{max: max} }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte{}, b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string { return string(b.buf) }

// errorLine returns the first line of s that mentions an error, else its last
// non-empty line. It is the only part of a tool's stderr that reaches a Cause.
func errorLine(s string) string {
	last := ""
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		last = l
		if strings.Contains(strings.ToLower(l), "error") {
			return l
		}
	}
	return last
}
