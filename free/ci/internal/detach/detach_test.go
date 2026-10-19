package detach

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const queuedLine = `{"schema_version":"1","command":"ci","data":{"type":"queued","schema":"ci.events/v1","seq":1,"ts":"2026-10-19T00:00:00Z","run_id":"r20261019T000000-abcd1234","pipeline_id":"r20261019T000000-abcd1234","data":{}}}`

func golden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if *update {
		if err = os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s differs (go test -update rewrites it):\n%s", name, got)
	}
}

func TestHandleGolden(t *testing.T) {
	golden(t, "handle.local.golden.json", NewHandle("r1", 4242, "", "/state/detached/r1.ndjson", ""))
	h := NewHandle("r1", 4242, "https://ci.example.test:8443", "/state/detached/r1.ndjson", "build")
	golden(t, "handle.coordinator.golden.json", h)
	if !reflect.DeepEqual(h.Commands.Wait[len(h.Commands.Wait)-2:], []string{"--coordinator", "https://ci.example.test:8443"}) ||
		!reflect.DeepEqual(h.Commands.Follow[len(h.Commands.Follow)-2:], []string{"--coordinator", "https://ci.example.test:8443"}) {
		t.Fatalf("wait and follow must carry --coordinator: %v %v", h.Commands.Wait, h.Commands.Follow)
	}
	for _, c := range [][]string{h.Commands.Evidence, h.Commands.Cancel, h.Commands.Logs} {
		if strings.Contains(strings.Join(c, " "), "--coordinator") {
			t.Fatalf("only wait and follow carry --coordinator: %v", c)
		}
	}
}

func TestChildArgs(t *testing.T) {
	for _, c := range []struct{ in, want []string }{
		{[]string{"--detach"}, []string{"--json", "--stream"}},
		{[]string{"--detach", "--job", "build", "--json"}, []string{"--json", "--stream", "--job", "build"}},
		{[]string{"-detach=true", "--pipeline", "x", "-stream"}, []string{"--json", "--stream", "--pipeline", "x"}},
		{[]string{"--detach", "--json=false", "--stream=false", "-job=x"}, []string{"--json", "--stream", "-job=x"}},
		{[]string{"--detach", "--json", "/path/to/repo"}, []string{"--json", "--stream", "/path/to/repo"}},
		{[]string{"--detach", "--", "go", "test", "./..."}, []string{"--json", "--stream", "--", "go", "test", "./..."}},
		{[]string{"--", "echo", "--detach", "--json"}, []string{"--json", "--stream", "--", "echo", "--detach", "--json"}},
		{[]string{"--detach", "--coordinator", "http://127.0.0.1:1"}, []string{"--json", "--stream", "--coordinator", "http://127.0.0.1:1"}},
	} {
		if got := ChildArgs(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ChildArgs(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCoordinatorOf(t *testing.T) {
	env := func(k string) string {
		if k == EnvCoordinator {
			return "https://from-env.example.test"
		}
		return ""
	}
	none := func(string) string { return "" }
	for _, c := range []struct {
		args []string
		env  func(string) string
		want string
	}{
		{[]string{"--detach"}, none, "local"},
		{[]string{"--detach", "--coordinator", "http://127.0.0.1:9"}, none, "http://127.0.0.1:9"},
		{[]string{"--detach", "-coordinator=http://127.0.0.1:9"}, env, "http://127.0.0.1:9"},
		{[]string{"--detach"}, env, "https://from-env.example.test"},
		{[]string{"--detach", "--", "run", "--coordinator", "x"}, none, "local"},
		{[]string{"--coordinator"}, none, "local"},
	} {
		if got := CoordinatorOf(c.args, c.env); got != c.want {
			t.Errorf("CoordinatorOf(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestChildEnvDropsToken(t *testing.T) {
	got := ChildEnv([]string{"A=1", EnvMCPToken + "=fake-mcp-token-value", "NSELF_MCP_TOKEN_FILE=/x", "B=2"})
	if !reflect.DeepEqual(got, []string{"A=1", "NSELF_MCP_TOKEN_FILE=/x", "B=2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseLine(t *testing.T) {
	rec, ok := ParseLine([]byte(queuedLine))
	if !ok || rec.Type != "queued" || rec.Seq != 1 || rec.RunID != "r20261019T000000-abcd1234" || rec.Final() {
		t.Fatalf("queued: %+v %v", rec, ok)
	}
	end, _ := ParseLine([]byte(`{"schema_version":"1","command":"ci","data":{"type":"result","schema":"ci.events/v1","seq":9,"run_id":"r"}}`))
	errLine, _ := ParseLine([]byte(`{"schema_version":"1","command":"ci","error":{"code":"E648","message":"m","exit_code":2,"class":"infra"}}`))
	if !end.Final() || !errLine.Final() || errLine.Exit != 2 {
		t.Fatalf("final lines: %+v %+v", end, errLine)
	}
	for _, junk := range []string{"", "plain stderr text", "{not json", `{"data":{}}`, `[1]`,
		`{"data":{"type":"queued","run_id":"fake-run"}}`,                                                   // no envelope version, no event schema
		`{"schema_version":"1","command":"ci","data":{"type":"queued","run_id":"fake-run","seq":1}}`,       // no event schema
		`{"schema_version":"1","command":"ci","error":{"code":"oops","message":"x","exit_code":1}}`,        // not a registered-shape code
		`{"schema_version":"1","command":"ci","data":{"type":"queued","schema":"ci.events/v1","seq":0}}`} { // seq starts at 1
		if _, ok := ParseLine([]byte(junk)); ok {
			t.Errorf("%q parsed", junk)
		}
	}
}

// fakeSpawn writes lines to the log path, then reports the child as exited when exit is set.
func fakeSpawn(lines []string, exit bool, started *[]string) func(argv, env []string, logPath string) (*ciexec.Detached, error) {
	return func(argv, env []string, logPath string) (*ciexec.Detached, error) {
		*started = append(append([]string{}, argv...), env...)
		if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			return nil, err
		}
		done := make(chan struct{})
		if exit {
			close(done)
		}
		return &ciexec.Detached{PID: 4242, Exited: done}, nil
	}
}

func TestStartQueued(t *testing.T) {
	home := t.TempDir()
	var started []string
	h, err := Start(Params{Exe: "/bin/nself-ci", Args: []string{"--detach", "--job", "b"}, Home: home,
		Env: []string{"A=1", EnvMCPToken + "=fake-mcp-token-value"}, Spawn: fakeSpawn([]string{"warning on stderr", queuedLine}, false, &started)})
	if err != nil {
		t.Fatal(err)
	}
	if h.Schema != Schema || h.RunID != "r20261019T000000-abcd1234" || h.State != "queued" || h.PID != 4242 || h.Coordinator != Local {
		t.Fatalf("handle %+v", h)
	}
	if want := filepath.Join(home, "detached", h.RunID+".ndjson"); h.StreamPath != want {
		t.Fatalf("stream path %s, want %s", h.StreamPath, want)
	}
	if _, err = os.Stat(h.StreamPath); err != nil {
		t.Fatalf("stream file was not renamed to the run id: %v", err)
	}
	if ReadPID(h.StreamPath) != 4242 {
		t.Fatalf("pid sidecar %d", ReadPID(h.StreamPath))
	}
	joined := strings.Join(started, " ")
	if !strings.HasPrefix(joined, "/bin/nself-ci --json --stream --job b") || strings.Contains(joined, "--detach") || strings.Contains(joined, EnvMCPToken) {
		t.Fatalf("child got %q", joined)
	}
	if got := StreamPath(home, h.RunID); got != h.StreamPath {
		t.Fatalf("StreamPath %q", got)
	}
}

func TestStartChildExitRedactsTail(t *testing.T) {
	const fake = "ghp_" + "0123456789abcdefghijklmnopqrstuvwxyz0123"
	const envSecret = "fixture-secret-value-0001"
	filler := strings.Repeat("x", 4000)
	var started []string
	_, err := Start(Params{Exe: "x", Args: []string{"--detach"}, Home: t.TempDir(), Env: []string{"FIXTURE_TOKEN=" + envSecret},
		Spawn: fakeSpawn([]string{filler, "boom: token " + fake + " and " + envSecret}, true, &started)})
	var de *Error
	if !errors.As(err, &de) || de.Reason != ReasonChildExit {
		t.Fatalf("want child_exit, got %v", err)
	}
	if strings.Contains(err.Error(), fake) || strings.Contains(err.Error(), envSecret) {
		t.Fatalf("secret in the error:\n%s", err)
	}
	if !strings.Contains(de.Tail, "boom: token") || len(de.Tail) > TailBytes+64 {
		t.Fatalf("tail %d bytes: %q", len(de.Tail), de.Tail)
	}
	if !strings.HasPrefix(err.Error(), "E648 child_exit: ") {
		t.Fatalf("message %q", err.Error())
	}
}

func TestStartNoQueuedStopsChild(t *testing.T) {
	var started []string
	var stopped int
	_, err := Start(Params{Exe: "x", Args: []string{"--detach"}, Home: t.TempDir(), Wait: 150 * time.Millisecond, Poll: 5 * time.Millisecond,
		Spawn: fakeSpawn([]string{"still starting"}, false, &started), Stop: func(pid int) error { stopped = pid; return nil }})
	var de *Error
	if !errors.As(err, &de) || de.Reason != ReasonNoQueued || stopped != 4242 || !strings.Contains(de.Tail, "still starting") {
		t.Fatalf("err %v stopped %d", err, stopped)
	}
}

func TestStartSpawnFails(t *testing.T) {
	_, err := Start(Params{Exe: "x", Args: nil, Home: t.TempDir(), Spawn: func([]string, []string, string) (*ciexec.Detached, error) {
		return nil, errors.New("exec: no such file")
	}})
	var de *Error
	if !errors.As(err, &de) || de.Reason != ReasonSpawn {
		t.Fatalf("want spawn, got %v", err)
	}
}

func TestErrorCodeInRange(t *testing.T) {
	if p := model.CodeProblems(); len(p) != 0 {
		t.Fatal(p)
	}
	for _, r := range []string{ReasonSpawn, ReasonNoQueued, ReasonChildExit} {
		if got := (&Error{Reason: r, Message: "m"}).Error(); got != "E648 "+r+": m" {
			t.Errorf("%q", got)
		}
	}
}

func TestJobOf(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--detach"}, ""},
		{[]string{"--detach", "--job", "build"}, "build"},
		{[]string{"--detach", "-job=test"}, "test"},
		{[]string{"--detach", "--", "go", "test"}, "adhoc"},
	} {
		if got := JobOf(c.args); got != c.want {
			t.Errorf("JobOf(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestTailRedactsAcrossTheCut(t *testing.T) {
	const secret = "fixture-secret-value-0001"
	// the secret ends 10 bytes before the end of the file and half of it lies inside the last
	// TailBytes: a plain 2 KiB cut would start inside it and leave its suffix unmasked
	path := filepath.Join(t.TempDir(), "s.ndjson")
	if err := os.WriteFile(path, []byte(strings.Repeat("y", TailBytes-len(secret)/2-10)+secret+"0123456789\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := RedactedTail(path, []string{"FIXTURE_TOKEN=" + secret})
	if strings.Contains(got, secret) || strings.Contains(got, secret[len(secret)/2:]) || len(got) > TailBytes {
		t.Fatalf("secret or its suffix survived (%d bytes): %q", len(got), got[:min(len(got), 80)])
	}
}

func TestStartVerifyRejectsAnUnknownRun(t *testing.T) {
	home := t.TempDir()
	var started []string
	_, err := Start(Params{Exe: "x", Home: home, Verify: func(string) bool { return false },
		Spawn: fakeSpawn([]string{queuedLine}, true, &started)})
	var de *Error
	if !errors.As(err, &de) || de.Reason != ReasonChildExit {
		t.Fatalf("want child_exit, got %v", err)
	}
	entries, _ := os.ReadDir(Dir(home))
	if len(entries) != 0 {
		t.Fatalf("the raw child output stayed on disk: %v", entries)
	}
}
