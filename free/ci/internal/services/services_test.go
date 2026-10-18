package services

// Purpose: sequence tests of the service engine against a stub Runner: the
// order of nself calls, error codes, credential hygiene, teardown on a failed
// start, and the stale-project sweep. No Docker and no nself binary needed.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// stub scripts the CLIs. up flips to true when a service is started and back
// to false when a stop (or compose down) ran, so leftover listings follow it.
type stub struct {
	mu       sync.Mutex
	calls    []string
	up       bool
	failOn   string // substring of a joined argv that exits 1
	noDocker bool
	noCLI    bool
	list     string
	echo     string // printed by every failing call
	removed  map[string]bool
}

func (s *stub) Run(_ context.Context, c Call) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	line := strings.Join(c.Argv, " ")
	s.calls = append(s.calls, line)
	switch {
	case s.failOn != "" && strings.Contains(line, s.failOn):
		return Result{ExitCode: 1, Output: s.echo}, nil
	case line == "docker info":
		if s.noDocker {
			return Result{ExitCode: 1}, nil
		}
	case strings.HasSuffix(line, "config services --help"):
		if s.noCLI {
			return Result{ExitCode: 1}, nil
		}
	case strings.HasSuffix(line, "config services list --json"):
		return Result{Output: s.list}, nil
	case strings.Contains(line, "config services start"):
		s.up = true
	case strings.Contains(line, " stop --volumes") || strings.Contains(line, "compose -p"):
		s.up = false
	case strings.HasPrefix(line, "docker ps -a -q") && s.up:
		return Result{Output: "abc123\n"}, nil
	}
	return Result{}, nil
}

func (s *stub) joined() string { s.mu.Lock(); defer s.mu.Unlock(); return strings.Join(s.calls, "\n") }

const listJSON = `[{"service":"redis","enabled":false,"status":"disabled","port":"6379","env_var":"REDIS_ENABLED"},{"service":"minio","enabled":false}]`

func newProv(t *testing.T, s *stub) *Provisioner {
	t.Helper()
	if s.list == "" {
		s.list = listJSON
	}
	return &Provisioner{Run: s, Home: t.TempDir(), Ready: func(context.Context, string) error { return nil }}
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestUpSequenceNamedComponentsOnly(t *testing.T) {
	s := &stub{}
	p := newProv(t, s)
	h, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Down(context.Background(), h) }()
	got := s.joined()
	order := []string{"docker info", "config services --help", "init --non-interactive --name " + h.Project,
		"config set POSTGRES_PORT", "config set POSTGRES_PASSWORD", "build", "config services start postgres"}
	at := 0
	for _, want := range order {
		i := strings.Index(got[at:], want)
		if i < 0 {
			t.Fatalf("missing or out of order %q in:\n%s", want, got)
		}
		at += i + len(want)
	}
	if strings.Contains(got, "services start redis") || strings.Contains(got, "services enable") {
		t.Fatalf("started or enabled more than the named required component:\n%s", got)
	}
	if !strings.HasPrefix(h.Project, "nselfci-") || filepath.Base(filepath.Dir(h.Dir)) != h.Project {
		t.Fatalf("project %q dir %q", h.Project, h.Dir)
	}
	if h.Env["NSELF_CI_SERVICE_DB_HOST"] != "127.0.0.1" || h.Env["NSELF_CI_SERVICE_DB_NET_HOST"] != "postgres" || h.Env["NSELF_CI_SERVICE_DB_NET_PORT"] != "5432" {
		t.Fatalf("env: %v", h.Env)
	}
	if !strings.HasPrefix(h.Env["NSELF_CI_SERVICE_DB_URL"], "postgres://nselfci:") || !strings.HasSuffix(h.Env["NSELF_CI_SERVICE_DB_URL"], "/nselfci") {
		t.Fatalf("url: %s", h.Env["NSELF_CI_SERVICE_DB_URL"])
	}
}

func TestOptionalComponentIsEnabled(t *testing.T) {
	s := &stub{}
	p := newProv(t, s)
	h, err := p.Up(context.Background(), []Spec{{Name: "cache", Component: "redis"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Down(context.Background(), h) }()
	if !strings.Contains(s.joined(), "config services enable redis") || !strings.Contains(s.joined(), "config services start redis") {
		t.Fatalf("redis not enabled and started:\n%s", s.joined())
	}
}

func TestErrorCodes(t *testing.T) {
	cases := []struct {
		name  string
		s     *stub
		specs []Spec
		want  string
	}{
		{"no docker", &stub{noDocker: true}, []Spec{{Name: "postgres"}}, CodeNoDocker},
		{"old cli", &stub{noCLI: true}, []Spec{{Name: "postgres"}}, CodeNoCLI},
		{"unsupported component", &stub{}, []Spec{{Name: "x", Component: "kafka"}}, CodeUnknown},
		{"cli lacks component", &stub{list: `[{"service":"minio"}]`}, []Spec{{Name: "cache", Component: "redis"}}, CodeUnknown},
		{"duplicate name", &stub{}, []Spec{{Name: "db", Component: "postgres"}, {Name: "DB", Component: "redis"}}, CodeUnknown},
		{"no services", &stub{}, nil, CodeUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := newProv(t, c.s).Up(context.Background(), c.specs)
			if code(err) != c.want {
				t.Fatalf("got %v, want %s", err, c.want)
			}
			if strings.Contains(c.s.joined(), "config services start") {
				t.Fatal("started a service after a refusal")
			}
		})
	}
}

func TestFailedStartTearsDownAndHidesCredentials(t *testing.T) {
	s := &stub{failOn: "config services start", echo: "port busy"}
	p := newProv(t, s)
	_, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if code(err) != CodeProvision {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(s.joined(), "stop --volumes --remove-orphans") {
		t.Fatalf("no teardown after a failed start:\n%s", s.joined())
	}
	work := filepath.Join(p.Home, "work")
	if entries, _ := os.ReadDir(work); len(entries) != 0 {
		t.Fatalf("work directory not empty: %v", entries)
	}
}

func TestCredentialsNeverInErrors(t *testing.T) {
	var pw string
	s := &stub{}
	p := newProv(t, s)
	p.Secret = func(v string) { pw = v }
	s.failOn, s.echo = "config services start", "boom: "
	// The failing call's output must contain the password: wrap Run so it learns it.
	wrapped := &echoPassword{stub: s, pw: &pw}
	p.Run = wrapped
	_, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err == nil || pw == "" {
		t.Fatalf("expected a failure with a registered secret, got %v / %q", err, pw)
	}
	if strings.Contains(err.Error(), pw) {
		t.Fatalf("error leaks the credential: %v", err)
	}
}

type echoPassword struct {
	*stub
	pw *string
}

func (e *echoPassword) Run(ctx context.Context, c Call) (Result, error) {
	r, err := e.stub.Run(ctx, c)
	if r.ExitCode != 0 {
		r.Output += *e.pw
	}
	return r, err
}

func TestDownIsIdempotentAndReportsLeftovers(t *testing.T) {
	s := &stub{}
	p := newProv(t, s)
	h, err := p.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Down(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if err := p.Down(context.Background(), h); err != nil {
		t.Fatalf("second Down: %v", err)
	}
	if _, err := os.Stat(h.root + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock file left behind: %v", err)
	}
	// A stuck project: compose down does not clear the containers.
	stuck := &stuckStub{stub: &stub{}}
	p2 := newProv(t, stuck.stub)
	p2.Run = stuck
	h2, err := p2.Up(context.Background(), []Spec{{Name: "db", Component: "postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.Down(context.Background(), h2); code(err) != CodeProvision || !strings.Contains(err.Error(), "left behind") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(h2.root + ".lock"); err != nil {
		t.Fatalf("lock file must stay for the next sweep: %v", err)
	}
}

type stuckStub struct{ *stub }

func (s *stuckStub) Run(ctx context.Context, c Call) (Result, error) {
	if strings.HasPrefix(strings.Join(c.Argv, " "), "docker ps -a -q") {
		return Result{Output: "stuck\n"}, nil
	}
	return s.stub.Run(ctx, c)
}

func TestSweepRemovesOnlyDeadOwners(t *testing.T) {
	s := &stub{}
	p := newProv(t, s)
	work := filepath.Join(p.Home, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	live, held, err := acquireLock(filepath.Join(work, "nselfci-aaaaaaaa.lock"))
	if err != nil || held {
		t.Fatal(err, held)
	}
	defer live.Unlock()
	for _, n := range []string{"nselfci-bbbbbbbb", "nselfci-cccccccc"} { // owners that died: lock files nobody holds
		if err := os.MkdirAll(filepath.Join(work, n, "svc"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, n+".lock"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	n, err := p.Sweep(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("swept %d, err %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(work, "nselfci-aaaaaaaa.lock")); err != nil {
		t.Fatal("a live owner's lock file was removed")
	}
	if strings.Contains(s.joined(), "nselfci-aaaaaaaa") {
		t.Fatal("touched a live owner's project")
	}
	for _, n := range []string{"nselfci-bbbbbbbb", "nselfci-cccccccc"} {
		if _, err := os.Stat(filepath.Join(work, n)); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", n)
		}
	}
}

func TestComponentNamesShapes(t *testing.T) {
	for name, out := range map[string]string{
		"array":    `[{"service":"postgres"},{"service":"redis"}]`,
		"envelope": `{"ok":true,"data":{"services":[{"service":"postgres"},{"name":"redis"}]}}`,
		"warning":  "DEPRECATED: x\n[{\"service\":\"postgres\"},{\"service\":\"redis\"}]\n",
	} {
		got := strings.Join(ComponentNames(out), ",")
		if got != "postgres,redis" {
			t.Errorf("%s: %q", name, got)
		}
	}
	if ComponentNames("not json") != nil {
		t.Error("garbage must give no names")
	}
}

func TestPickPortsDistinctAndReleased(t *testing.T) {
	a, err := pickPorts(4)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, p := range a {
		if seen[p] || p < 1024 {
			t.Fatalf("bad ports %v", a)
		}
		seen[p] = true
	}
	releasePorts(a)
	portMu.Lock()
	defer portMu.Unlock()
	for _, p := range a {
		if portHeld[p] {
			t.Fatal("port still held after release")
		}
	}
}
