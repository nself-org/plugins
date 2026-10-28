//go:build unix

package signing

// Purpose: the unix FunnelRunner (see stdio.go).
// Inputs:  a StdioCall. Outputs: combined output and exit code.
// Constraints: the FIFO's directory is created 0700 under the system temp
// directory and removed on every path; the writer never blocks the call:
// when the child never opened the FIFO the read end is opened here once the
// funnel returned, which releases the writer; output is capped at maxOutput.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

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

// write keeps at most maxOutput bytes in total.
func (c *capture) write(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, b[:min(len(b), max(maxOutput-len(c.buf), 0))]...)
}

// mkfifo is replaced by tests.
var mkfifo = syscall.Mkfifo

// releasePoll is how often freeWriter opens the read end.
const releasePoll = time.Duration(1e7)

// freeWriter waits for the writer goroutine. A child that never opened the
// FIFO leaves the writer blocked in open, so the read end is opened and
// closed again and again until the writer is through (the writer may not have
// reached its open yet the first time); it gives up after about 5 seconds.
func freeWriter(fifo string, done <-chan struct{}) {
	for range 500 {
		select {
		case <-done:
			return
		case <-time.After(releasePoll):
			release(fifo)
		}
	}
}

// release opens and closes the read end of a FIFO once, which frees a writer
// blocked on a child that never opened it. It reports whether the open worked.
func release(fifo string) bool {
	rd, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	_ = rd.Close()
	return true
}

// RunStdio implements StdioRunner.
func (r *FunnelRunner) RunStdio(ctx context.Context, c StdioCall) ([]byte, int, error) {
	if len(c.Argv) == 0 {
		return nil, notRun, Failf(ReasonSignerFailed, "no program to run")
	}
	env := append([]string(nil), c.Env...)
	var fifo string
	writerDone := make(chan struct{})
	if c.Stdin == nil {
		close(writerDone) // no writer: freeWriter returns at once
	} else {
		dir, err := os.MkdirTemp("", "sigio-")
		if err != nil {
			return nil, notRun, Failf(ReasonSignerFailed, "no private directory for the request")
		}
		defer func() { _ = os.RemoveAll(dir) }()
		fifo = filepath.Join(dir, "in")
		if err := mkfifo(fifo, 0o600); err != nil {
			return nil, notRun, Failf(ReasonSignerFailed, "no pipe for the request")
		}
		env = append(env, "NSELF_CI_STDIN="+fifo)
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
	ex := &ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: extraEnv(env), NewRedactor: func() ciexec.Redactor { return passRedactor{} }}
	res := ex.Run(ctx, ciexec.JobSpec{AttemptID: r.id(), CoordinatorID: "signer", Command: argv, Home: r.Home, Timeout: c.Timeout, Output: cap.write})
	freeWriter(fifo, writerDone)
	if res.Err != nil {
		clear(cap.buf)
		return nil, notRun, Failf(ReasonSignerFailed, "the program could not be run or timed out")
	}
	return cap.buf, res.ExitCode, nil
}

func (r *FunnelRunner) id() string {
	return "sig-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(r.seq.Add(1), 10)
}
