package gitbatch

// Purpose: one long-lived `git cat-file --batch-command` per repository, so the object reads at the
// start of a run (HEAD, a tree entry, a blob, a commit's type) cost one process instead of one funnel
// spawn each (P7-CI-46: about twenty git launches before the first job, each with its own sh, workspace
// directory and process-tree polling).
// Inputs:  a repository root, the run's context and the request: an object name (`HEAD`, `<sha>:./path`).
// Outputs: Object (id, type, size) and, for Read, the object bytes.
// Constraints: the process starts through the exec funnel (internal/exec is the only place that starts
//          processes) with the environment allowlist (no GIT_DIR, GIT_CONFIG_*, GIT_OBJECT_DIRECTORY ...),
//          `--no-replace-objects`, `core.fsmonitor=false`, no lazy fetch and no terminal prompt, so it cannot
//          reach the network; cat-file only reads and runs no hook. Every request has a timeout; a timeout, a
//          malformed reply or a failed write kills the process and the read FAILS (never a guess). A session
//          that cannot answer at all (git too old for --batch-command, not a repository, ctx gone, oversized
//          object) reports ErrUnavailable and the caller asks the per-read git process instead, which is the
//          original check. An idle session exits by itself and the next read starts a new one.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Errors a read returns. ErrUnavailable is the only one a caller may answer by running git itself.
var (
	ErrUnavailable = errors.New("gitbatch: session unavailable")
	ErrNotFound    = errors.New("gitbatch: object not found")
	ErrAmbiguous   = errors.New("gitbatch: object name is ambiguous")
	ErrMalformed   = errors.New("gitbatch: malformed reply")
	ErrTimeout     = errors.New("gitbatch: read timed out")
)

// Object is what `info` says about an object.
type Object struct {
	OID  string
	Type string
	Size int64
}

// Options bound a session; zero values take the defaults.
type Options struct {
	ReadTimeout time.Duration // per request, default 10 s
	Idle        time.Duration // an unused process exits after this, default 3 s
	MaxBytes    int64         // largest object Read returns, default 64 MiB
}

func (o Options) withDefaults() Options {
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = 10 * time.Second
	}
	if o.Idle <= 0 {
		o.Idle = 3 * time.Second
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 64 << 20
	}
	return o
}

// Session answers object reads for one repository root.
type Session struct {
	root   string
	parent context.Context
	opt    Options

	mu       sync.Mutex // one request at a time; guards the fields below
	p        *proc
	poisoned bool
	closed   bool
	last     time.Time
	idleT    *time.Timer
}

// NewSession makes a session for root. The process starts on the first read and ends when parent ends.
func NewSession(parent context.Context, root string, opt Options) *Session {
	return &Session{root: root, parent: parent, opt: opt.withDefaults()}
}

// proc is one running `git cat-file`.
type proc struct {
	stdin   *os.File // request lines
	stdinR  *os.File // the child's end, closed when it has exited
	pr      *io.PipeReader
	rd      *bufio.Reader
	cancel  context.CancelFunc
	done    chan struct{}         // closed when the funnel run returned
	replied bool                  // at least one well-formed reply came back
	why     atomic.Pointer[error] // error that killed it
}

var seq atomic.Int64

// start launches git through the funnel.
func (s *Session) start() (*proc, error) {
	if s.parent.Err() != nil {
		return nil, ErrUnavailable
	}
	home, err := os.MkdirTemp("", "nself-ci-gb-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		_ = os.RemoveAll(home)
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(s.parent)
	p := &proc{stdin: stdinW, stdinR: stdinR, pr: pr, rd: bufio.NewReaderSize(pr, 64<<10), cancel: cancel, done: make(chan struct{})}
	ex := ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: ciexec.AllowlistEnv{}, Grace: 500 * time.Millisecond}
	spec := ciexec.JobSpec{
		AttemptID: fmt.Sprintf("gitbatch-%d-%d", time.Now().UnixNano(), seq.Add(1)), CoordinatorID: "gitbatch",
		Job:  model.Job{Kind: "static", Env: map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_NO_LAZY_FETCH": "1"}},
		Home: home,
		Command: []string{"git", "--no-replace-objects", "-c", "core.fsmonitor=false", "-C", s.root,
			"cat-file", "--batch-command"},
		Stdin:  stdinR,
		Output: func(b []byte) { _, _ = pw.Write(b) },
	}
	go func() {
		_ = ex.Run(ctx, spec)
		_ = pw.CloseWithError(io.EOF)
		_ = stdinR.Close()
		_ = os.RemoveAll(home)
		cancel()
		close(p.done)
	}()
	return p, nil
}

// kill ends the process and fails every pending and later read on it with why.
func (p *proc) kill(why error) {
	p.why.CompareAndSwap(nil, &why)
	_ = p.pr.CloseWithError(why)
	_ = p.stdin.Close()
	p.cancel()
}

// stop is the clean shutdown: EOF on stdin ends cat-file; the run is cancelled when it does not.
func (p *proc) stop() {
	_ = p.stdin.Close()
	select {
	case <-p.done:
		_ = p.pr.Close()
		return
	case <-time.After(time.Second):
	}
	p.kill(ErrUnavailable)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}

// dispose forgets the current process (after it died or was killed) and stops it.
func (s *Session) dispose() {
	if s.p != nil {
		s.p.kill(ErrUnavailable)
		s.p = nil
	}
}

// Close stops the process and makes later reads report ErrUnavailable. It is safe to call twice.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.idleT != nil {
		s.idleT.Stop()
	}
	if p := s.p; p != nil {
		s.p = nil
		p.stop()
	}
}

// closeIdle ends a process nobody used for the idle period.
func (s *Session) closeIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.p == nil || time.Since(s.last) < s.opt.Idle {
		return
	}
	p := s.p
	s.p = nil
	p.stop()
}

// Stat is `info <name>`: the id, type and size of the object name resolves to (any rev-parse expression).
func (s *Session) Stat(ctx context.Context, name string) (Object, error) {
	obj, _, err := s.do(ctx, "info", name)
	return obj, err
}

// Read is `contents <name>`: the object and its bytes. An object over Options.MaxBytes is ErrUnavailable.
func (s *Session) Read(ctx context.Context, name string) (Object, []byte, error) {
	return s.do(ctx, "contents", name)
}

func (s *Session) do(ctx context.Context, verb, name string) (Object, []byte, error) {
	if name == "" || strings.ContainsAny(name, "\n\r\x00") {
		return Object{}, nil, fmt.Errorf("%w: unsupported object name", ErrUnavailable) // would break the line framing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parent.Err() != nil { // the run is over: nothing here may answer, the caller asks git
		s.dispose()
		if ctx.Err() == nil {
			return Object{}, nil, ErrUnavailable
		}
	}
	if s.p != nil {
		select {
		case <-s.p.done: // exited by itself since the last read: start a fresh one
			s.dispose()
		default:
		}
	}
	if s.closed || s.poisoned || ctx.Err() != nil {
		if ctx.Err() != nil {
			return Object{}, nil, ctx.Err()
		}
		return Object{}, nil, ErrUnavailable
	}
	if s.p == nil {
		p, err := s.start()
		if err != nil {
			return Object{}, nil, err
		}
		s.p = p
	}
	p := s.p
	timer := time.AfterFunc(s.opt.ReadTimeout, func() { p.kill(ErrTimeout) })
	stopCtx := context.AfterFunc(ctx, func() { p.kill(ctx.Err()) })
	defer timer.Stop()
	defer stopCtx()
	obj, data, err := s.exchange(p, verb, name)
	switch {
	case err == nil:
		p.replied = true
		s.last = time.Now()
		if s.idleT == nil {
			s.idleT = time.AfterFunc(s.opt.Idle, s.closeIdle)
		} else {
			s.idleT.Reset(s.opt.Idle)
		}
		return obj, data, nil
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrAmbiguous):
		p.replied = true // a proper answer: the session is fine
		s.last = time.Now()
		return Object{}, nil, err
	}
	// The process is no longer in a known state: end it. What the caller sees depends on why.
	if why := p.why.Load(); why != nil && !errors.Is(*why, ErrUnavailable) {
		err = *why
	}
	replied := p.replied
	s.dispose()
	switch {
	case ctx.Err() != nil:
		return Object{}, nil, ctx.Err()
	case !replied && !errors.Is(err, ErrTimeout):
		s.poisoned = true                                               // do not relaunch a git that cannot serve this mode on every read
		return Object{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err) // git never answered: ask the real git
	case errors.Is(err, ErrTimeout), errors.Is(err, ErrMalformed):
		s.poisoned = true
	}
	return Object{}, nil, err
}
