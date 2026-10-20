package tart

// Purpose: start `tart` and `ssh` through the one process funnel (internal/exec), never through os/exec.
// Inputs:  an argv, optional stdin; for a long-lived VM process, an argv and a log path.
// Outputs: the command's combined output (capped) and exit code, or a Proc handle for the VM process.
// Constraints: this package imports no os/exec (layers rule). A short command is a nested exec.Executor run
// under the attempt's coordinator id, so the executor's own process markers and sweeps cover it; the VM
// process is started detached and is always stopped through `tart stop` plus Kill on teardown.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// maxToolOutput caps what one short command may return.
const maxToolOutput = 1 << 20

// Result is the outcome of a short command.
type Result struct {
	Out  []byte
	Exit int
}

// Proc is a started VM process.
type Proc interface {
	// Exited closes when the process has ended.
	Exited() <-chan struct{}
	// Kill ends the process group; it is safe to call after the process exited.
	Kill() error
}

// Tool starts processes for the profile; tests replace it.
type Tool interface {
	// Run runs argv to completion. A non-zero exit is a Result, not an error; err means it could not run.
	// Output goes to out; with a nil out it is returned in Result.Out, capped.
	Run(ctx context.Context, argv []string, stdin io.Reader, out io.Writer) (Result, error)
	// Start starts argv detached with its output appended to logPath.
	Start(argv []string, logPath string) (Proc, error)
}

type execTool struct{ coordinator, home string }

// NewExecTool returns the Tool backed by the exec funnel for one coordinator and CI home.
func NewExecTool(coordinator, home string) Tool { return execTool{coordinator, home} }

var toolSeq atomic.Uint64

type capBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *capBuffer) write(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := maxToolOutput - c.buf.Len(); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		c.buf.Write(p)
	}
}

func (c *capBuffer) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func (t execTool) Run(ctx context.Context, argv []string, stdin io.Reader, w io.Writer) (Result, error) {
	var out capBuffer
	sink := out.write
	if w != nil {
		sink = func(p []byte) { _, _ = w.Write(p) }
	}
	var ex exec.Executor // inherits the operator's environment: tart needs HOME and TART_HOME
	res := ex.Run(ctx, exec.JobSpec{
		AttemptID:     fmt.Sprintf("tart-%d-%d", time.Now().UnixNano(), toolSeq.Add(1)),
		CoordinatorID: t.coordinator,
		Job:           model.Job{Kind: "static"},
		Home:          t.home,
		Command:       argv,
		Stdin:         stdin,
		Output:        sink,
	})
	return Result{Out: out.bytes(), Exit: res.ExitCode}, res.Err
}

type detachedProc struct{ d *exec.Detached }

func (p detachedProc) Exited() <-chan struct{} { return p.d.Exited }

func (p detachedProc) Kill() error {
	select {
	case <-p.d.Exited:
		return nil
	default:
		return exec.TerminateDetached(p.d.PID)
	}
}

func (t execTool) Start(argv []string, logPath string) (Proc, error) {
	d, err := exec.StartDetached(argv, os.Environ(), logPath)
	if err != nil {
		return nil, err
	}
	return detachedProc{d}, nil
}

// stateDir is where the profile keeps per-clone markers, ssh configs and VM logs.
func stateDir(home string) string { return filepath.Join(home, "tart") }
