// Purpose: a fake Exec for the ops unit tests: it plays the postgres container.
//
// Inputs: canned answers (snapshot, fingerprint lines, dump bytes, restore stderr).
//
// Outputs: records every call (argv and stdin) so tests can assert order, argv
// contents and that nothing ran.
//
// Constraints: no process, no network, no docker.
package ops

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
)

type call struct {
	kind  string // psql-session, psql, pg_dump, pg_restore
	argv  []string
	stdin string
}

type fakeExec struct {
	mu       sync.Mutex
	calls    []call
	sessions []*fakeSession

	snapshot string
	version  string
	fps      []string // lines the fingerprint query prints in the backup session
	postFps  []string // lines the fingerprint query prints after a restore
	psqlDies bool     // the interactive session ends before the sentinel
	psqlErr  string   // its stderr

	dump       []byte
	dumpErr    error // pg_dump fails after writing dump
	restoreErr string
	restoreRC  error
	restored   []byte
}

func newFake() *fakeExec {
	return &fakeExec{
		snapshot: "00000003-0000001B-1", version: "160004",
		fps:     []string{`{"schema":"auth","table":"users","rows":2,"sum":"11"}`, `{"schema":"public","table":"t","rows":1000,"sum":"-5"}`, `{"schema":"storage","table":"files","rows":0,"sum":"0"}`},
		dump:    []byte("PGDMP-fake-dump-bytes"),
		postFps: nil,
	}
}

func (f *fakeExec) record(c call) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

func (f *fakeExec) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.kind == kind {
			n++
		}
	}
	return n
}

func kindOf(argv []string) string {
	s := strings.Join(argv, " ")
	switch {
	case strings.Contains(s, "exec pg_dump"):
		return "pg_dump"
	case strings.Contains(s, "exec pg_restore"):
		return "pg_restore"
	}
	return "psql"
}

func (f *fakeExec) Run(_ context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	k := kindOf(argv)
	in := ""
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
		if k == "pg_restore" {
			f.restored = b
			in = "<dump>"
		}
	}
	f.record(call{kind: k, argv: argv, stdin: in})
	switch k {
	case "pg_dump":
		_, _ = stdout.Write(f.dump)
		return f.dumpErr
	case "pg_restore":
		_, _ = io.WriteString(stderr, f.restoreErr)
		return f.restoreRC
	}
	for _, l := range f.postFps {
		_, _ = io.WriteString(stdout, l+"\n")
	}
	return nil
}

func (f *fakeExec) Start(_ context.Context, argv []string, stderr io.Writer) (Session, error) {
	s := &fakeSession{f: f, ready: make(chan string, 1), quit: make(chan struct{})}
	s.out, s.outW = io.Pipe()
	s.in = &fakeStdin{s: s}
	_, _ = io.WriteString(stderr, f.psqlErr)
	f.mu.Lock()
	f.sessions = append(f.sessions, s)
	f.mu.Unlock()
	go s.play()
	f.record(call{kind: "psql-session", argv: argv})
	return s, nil
}

type fakeSession struct {
	f     *fakeExec
	in    *fakeStdin
	out   *io.PipeReader
	outW  *io.PipeWriter
	ready chan string // the sentinel, once the script was written
	quit  chan struct{}
	once  sync.Once
}

func (s *fakeSession) stop() { s.once.Do(func() { close(s.quit) }) }

type fakeStdin struct {
	s      *fakeSession
	mu     sync.Mutex
	buf    strings.Builder
	closed bool
}

func (w *fakeStdin) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("closed")
	}
	w.buf.Write(p)
	if i := strings.Index(w.buf.String(), "\\echo NSELF"); i >= 0 {
		select {
		case w.s.ready <- strings.TrimSpace(strings.SplitN(w.buf.String()[i+6:], "\n", 2)[0]):
		default:
		}
	}
	return len(p), nil
}

func (w *fakeStdin) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.s.stop()
	return nil
}

func (w *fakeStdin) text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (s *fakeSession) play() {
	if s.f.psqlDies {
		_ = s.outW.Close()
		return
	}
	var sentinel string
	select {
	case sentinel = <-s.ready:
	case <-s.quit:
		_ = s.outW.Close()
		return
	}
	_, _ = io.WriteString(s.outW, s.f.snapshot+"\n"+s.f.version+"\n")
	for _, l := range s.f.fps {
		_, _ = io.WriteString(s.outW, l+"\n")
	}
	_, _ = io.WriteString(s.outW, sentinel+"\n")
	// Stay open until the stdin is closed or the session is killed, like psql.
	<-s.quit
	_ = s.outW.Close()
}

func (s *fakeSession) Stdin() io.WriteCloser { return s.in }
func (s *fakeSession) Stdout() io.Reader     { return s.out }
func (s *fakeSession) Wait() error {
	if s.f.psqlDies {
		return errors.New("exit status 3")
	}
	return nil
}
func (s *fakeSession) Kill() { s.stop(); _ = s.outW.Close() }
