package exec

// Purpose: a job's payload must not run until the agent has recorded the job's
// process marker (D-1531). The marker is what the orphan sweep reads; a job
// started first and marked after could, if the agent is SIGKILLed in between,
// run on with no marker and never be found.
// Inputs:  two pipes whose read ends the job's gate helper inherits: fd 3 for the
// go-ahead and fd 4 for the payload's environment.
// Outputs: release (write "g\n" and close) once the marker is on disk.
// Constraints: the helper (the agent binary re-executed with GateArg as its
// first argument, see process_gate_unix.go) execs the payload only after reading
// that line; end of file with no line (the agent died, or gave up) makes it exit
// 125 without running the payload. The write end is close-on-exec and never
// reaches the job. No shell is involved, nothing is added to the payload's
// environment, and the helper itself runs with a fixed environment of its own.

import (
	"errors"
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
	r, w   *os.File // fd 3 in the helper: the go-ahead line
	er, ew *os.File // fd 4 in the helper: the payload's environment, NUL-terminated entries
	blob   []byte
	sent   chan error
	once   sync.Once
}

// newStartGate returns a nil gate on a host that cannot gate (the job then starts ungated). The payload's
// environment is not given to the helper process (a Go runtime reads GOMEMLIMIT, GODEBUG and others from its
// own environment before main): it travels on fd 4. It is what a direct start would hand the payload,
// including the dedup and the PWD that os/exec applies, so the payload sees exactly the same entries.
func newStartGate(s JobSpec, workdir string, env []string) (*startGate, []string, error) {
	argv, ok, err := gatedCommand(s, workdir, env)
	if err != nil || !ok {
		return nil, argv, err
	}
	// Validate the environment as a direct start does before building it: os/exec refuses an entry with a NUL
	// byte (its Environ drops such entries without a word, so the check must come first), and the gated job
	// must fail with that same error, not run with variables missing.
	for _, e := range env {
		if strings.IndexByte(e, 0) >= 0 {
			return nil, nil, errors.New("exec: environment variable contains NUL")
		}
	}
	var blob []byte
	for _, e := range (&osexec.Cmd{Dir: workdir, Env: env}).Environ() {
		blob = append(append(blob, e...), 0)
	}
	g := &startGate{blob: blob, sent: make(chan error, 1)}
	if g.r, g.w, err = os.Pipe(); err != nil {
		return nil, nil, err
	}
	if g.er, g.ew, err = os.Pipe(); err != nil {
		_ = g.r.Close()
		_ = g.w.Close()
		return nil, nil, err
	}
	return g, argv, nil
}

// helperEnv is the helper process's own environment: fixed, and nothing from the job (os/exec adds PWD).
func (g *startGate) helperEnv(job []string) []string {
	if g == nil {
		return job
	}
	return []string{}
}

// extra is what the child inherits: fd 3 the go-ahead line, fd 4 the payload environment.
func (g *startGate) extra() []*os.File {
	if g == nil {
		return nil
	}
	return []*os.File{g.r, g.er}
}

// begin starts writing the payload environment to the helper. It runs in its own goroutine, so an
// environment larger than the pipe buffer cannot block the agent against a helper that has not read yet.
func (g *startGate) begin() {
	if g == nil {
		return
	}
	go func() {
		_, err := g.ew.Write(g.blob)
		if cerr := g.ew.Close(); err == nil {
			err = cerr
		}
		g.sent <- err
	}()
}

// release lets the payload run once the marker is on disk and the whole environment has been written. The
// helper reads the environment to end of file before it looks for the go-ahead, so a go-ahead is only ever
// seen with a complete environment.
func (g *startGate) release() (err error) {
	if g == nil {
		return nil
	}
	g.once.Do(func() {
		if err = <-g.sent; err != nil {
			_ = g.w.Close()
			return
		}
		_, err = g.w.Write([]byte("g\n"))
		if cerr := g.w.Close(); err == nil {
			err = cerr
		}
	})
	return err
}

// abandon closes every write end with no go-ahead: a job still waiting exits without running its payload.
// It is safe after release and on a nil gate.
func (g *startGate) abandon() {
	if g == nil {
		return
	}
	g.once.Do(func() { _ = g.w.Close() })
	_ = g.ew.Close()
	_ = g.r.Close() // no-ops after the child started (runCommandFiles closed them); free them when Start never ran
	_ = g.er.Close()
}

// resolveGatedPath finds the payload before the job starts, so a missing or non-executable binary is a start
// error (as cmd.Start reports it) and not a failure inside the helper. A name with a slash is checked as
// given (relative to the job directory). A bare name is looked up on the job's effective PATH only: the last
// PATH= in env, with no fallback to the agent's PATH. Only when env has no PATH at all is the agent's used,
// which is what a direct start of the same command does.
func resolveGatedPath(name, dir string, env []string) (string, error) {
	if strings.ContainsRune(name, '/') {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return p, executableFile(p)
	}
	path, ok := os.LookupEnv("PATH")
	for _, e := range env {
		if v, found := strings.CutPrefix(e, "PATH="); found {
			path, ok = v, true
		}
	}
	if ok {
		for _, d := range filepath.SplitList(path) {
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
