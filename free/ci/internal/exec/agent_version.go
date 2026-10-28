package exec

// Purpose: the narrow seam `nself-ci-agent upgrade` (D-1385) needs from the one package that may start processes: run a
// candidate agent binary as `<bin> version` and read what it prints, so an upgrade learns that a new binary starts before it is
// left in place instead of at the next start.
// Inputs:  the absolute path of an agent binary.
// Outputs: the binary's stdout (capped at maxAgentVersionOutput bytes); an E610 error when it cannot start, exits non-zero,
// runs past agentVersionTimeout or prints more than the cap.
// Constraints: argv is the one fixed word `version`; the child gets an empty working directory's worth of environment (PATH,
// TMPDIR and, on Windows, SystemRoot), no stdin and no inherited secrets; a hung binary is killed when the time box ends; nothing
// is parsed or decided here (internal/agent/update compares the version).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"time"
)

const (
	// agentVersionTimeout bounds one `version` run: it prints one line, so a binary that needs longer is broken.
	agentVersionTimeout = 10 * time.Second
	// maxAgentVersionOutput bounds what a candidate binary can make the caller hold.
	maxAgentVersionOutput = 4 << 10
)

// cappedBuffer keeps at most max bytes and fails the child's write after that, which also ends a binary that prints for ever
// (the child then dies on the closed pipe, so the caller reads over, not the exit error).
type cappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.max {
		c.over = true
		return 0, errors.New("output over the cap")
	}
	return c.buf.Write(p)
}

// RunAgentVersion runs `<bin> version` and returns its stdout.
func RunAgentVersion(ctx context.Context, bin string) (string, error) {
	if !filepath.IsAbs(bin) {
		return "", coded("E610", "agent binary path must be absolute")
	}
	ctx, cancel := context.WithTimeout(ctx, agentVersionTimeout)
	defer cancel()
	out := &cappedBuffer{max: maxAgentVersionOutput}
	cmd := osexec.CommandContext(ctx, bin, "version")
	cmd.Dir = os.TempDir()
	cmd.Env = agentVersionEnv()
	cmd.Stdout = out
	cmd.WaitDelay = time.Second // a child that keeps the pipe open after the kill must not hold the caller
	if err := cmd.Run(); err != nil {
		switch {
		case ctx.Err() != nil:
			return "", &Error{Code: "E610", Message: "agent binary did not answer `version` in time", Err: ctx.Err()}
		case out.over:
			return "", &Error{Code: "E610", Message: fmt.Sprintf("agent binary printed more than %d bytes for `version`", maxAgentVersionOutput), Err: err}
		}
		return "", &Error{Code: "E610", Message: "agent binary failed `version`", Err: err}
	}
	return out.buf.String(), nil
}

func agentVersionEnv() []string {
	var env []string
	for _, key := range []string{"PATH", "TMPDIR", "SystemRoot"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
