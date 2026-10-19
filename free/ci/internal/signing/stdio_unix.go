//go:build unix

package signing

// Purpose: the unix FunnelRunner (see stdio.go).
// Inputs:  a StdioCall. Outputs: combined output and exit code.
// Constraints: the FIFO's directory is created 0700 under the funnel home
// (or the user cache), removed on every path; the writer never blocks the
// call: when the child never opened the FIFO the read end is opened here once
// the funnel returned, which releases the writer.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// FunnelRunner is the production StdioRunner.
type FunnelRunner struct {
	// Home is the CI state directory the funnel works under ("" = its default).
	Home string
	seq  atomic.Uint64
}

type capture struct {
	mu  sync.Mutex
	buf []byte
}

func (c *capture) write(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.buf) <= maxOutput {
		c.buf = append(c.buf, b...)
	}
}

// RunStdio implements StdioRunner.
func (r *FunnelRunner) RunStdio(ctx context.Context, c StdioCall) ([]byte, int, error) {
	if len(c.Argv) == 0 {
		return nil, -1, Failf(ReasonSignerFailed, "no program to run")
	}
	env := append([]string(nil), c.Env...)
	var fifo string
	var writerDone chan struct{}
	if c.Stdin != nil {
		dir, err := os.MkdirTemp(r.fifoParent(), "sigio-")
		if err != nil {
			return nil, -1, Failf(ReasonSignerFailed, "no private directory for the request")
		}
		defer func() { _ = os.RemoveAll(dir) }()
		fifo = filepath.Join(dir, "in")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			return nil, -1, Failf(ReasonSignerFailed, "no pipe for the request")
		}
		env = append(env, "NSELF_CI_STDIN="+fifo)
		writerDone = make(chan struct{})
		go func() {
			defer close(writerDone)
			if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
				_, _ = w.Write(c.Stdin)
				_ = w.Close()
			}
		}()
	}
	dir := c.Dir
	if dir == "" {
		dir = "."
	}
	argv := append([]string{"/bin/sh", "-c", wrapper, "sh", dir}, c.Argv...)
	var cap capture
	ex := &ciexec.Executor{Env: extraEnv(env), NewRedactor: func() ciexec.Redactor { return passRedactor{} }}
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: r.id(), CoordinatorID: "signer", Command: argv, Home: r.Home, Timeout: c.Timeout, Output: cap.write})
	if writerDone != nil {
		if rd, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil { // frees a writer the child never met
			_ = rd.Close()
		}
		<-writerDone
	}
	if res.Err != nil {
		clear(cap.buf)
		return nil, -1, Failf(ReasonSignerFailed, "the program could not be run or timed out")
	}
	return cap.buf, res.ExitCode, nil
}

func (r *FunnelRunner) fifoParent() string {
	if r.Home != "" {
		if err := os.MkdirAll(r.Home, 0o700); err == nil {
			return r.Home
		}
	}
	return ""
}

func (r *FunnelRunner) id() string {
	return "sig-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(r.seq.Add(1), 10)
}
