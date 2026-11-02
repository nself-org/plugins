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
	osexec "os/exec"
	"path/filepath"
	"strings"
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
func newStartGate(s JobSpec, workdir string, env []string) (*startGate, []string, error) {
	argv, ok, err := gatedCommand(s, workdir, env)
	if err != nil || !ok {
		return nil, argv, err
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

// resolveGatedPath finds the payload before the job starts, so a missing or non-executable binary is a start
// error (as cmd.Start reports it) and not a failure inside the shell. A bare name is looked up on the job's
// PATH, then the agent's; a name with a slash is checked as given (relative to the job directory).
func resolveGatedPath(name, dir string, env []string) (string, error) {
	if strings.ContainsRune(name, '/') {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return name, executableFile(p)
	}
	var paths []string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			paths = append(paths, filepath.SplitList(v)...)
		}
	}
	if v, ok := os.LookupEnv("PATH"); ok {
		paths = append(paths, filepath.SplitList(v)...)
	}
	for _, d := range paths {
		if d == "" {
			d = "."
		}
		if !filepath.IsAbs(d) {
			d = filepath.Join(dir, d)
		}
		p := filepath.Join(d, name)
		if executableFile(p) == nil {
			return p, nil
		}
	}
	return "", &osexec.Error{Name: name, Err: osexec.ErrNotFound}
}

func executableFile(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return &os.PathError{Op: "exec", Path: p, Err: os.ErrPermission}
	}
	return nil
}
