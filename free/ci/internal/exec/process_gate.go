package exec

// Purpose: a job's payload must not run until the agent has recorded the job's
// process marker (D-1531). The marker is what the orphan sweep reads; a job
// started first and marked after could, if the agent is SIGKILLed in between,
// run on with no marker and never be found.
// Inputs:  a pipe whose read end the job's shell inherits as fd 3.
// Outputs: release (write "g\n" and close) once the marker is on disk.
// Constraints: the shell runs the payload only after reading that line; end of
// file with no line (the agent died, or gave up) makes it exit without running
// the payload. The write end is close-on-exec and never reaches the job.

import (
	"os"
	"sync"
)

// BeforeMarkerHook is a test seam: when set it runs in the agent after the job
// process has started and before its marker is written. Production leaves it nil.
var BeforeMarkerHook func(pid int)

type startGate struct {
	r, w *os.File
	once sync.Once
}

// newStartGate returns nil on a host that cannot gate (the job then starts ungated).
func newStartGate(s JobSpec) (*startGate, []string, error) {
	argv, ok := gatedCommand(s)
	if !ok {
		return nil, argv, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return &startGate{r: r, w: w}, argv, nil
}

// extra is the read end the child inherits as fd 3.
func (g *startGate) extra() []*os.File {
	if g == nil {
		return nil
	}
	return []*os.File{g.r}
}

// release lets the payload run: the marker is on disk.
func (g *startGate) release() (err error) {
	if g == nil {
		return nil
	}
	g.once.Do(func() {
		_, err = g.w.Write([]byte("g\n"))
		if cerr := g.w.Close(); err == nil {
			err = cerr
		}
	})
	return err
}

// abandon closes the write end with no go-ahead: a job still waiting exits without running its payload.
// It is safe after release and on a nil gate.
func (g *startGate) abandon() {
	if g == nil {
		return
	}
	g.once.Do(func() { _ = g.w.Close() })
	_ = g.r.Close() // a no-op after the child started (runCommandFiles closed it); frees it when Start never ran
}
