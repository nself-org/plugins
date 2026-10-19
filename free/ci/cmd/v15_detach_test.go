package main

// Tests of `nself ci --detach` (P7-CI-63): the handle, the child argv and
// environment, ad-hoc and coordinator forms, E648 with a redacted tail, and
// stream cleanup by `runs prune`. The detached child is this test binary in a
// fake-child mode (TestMain), so the real spawn path runs without an engine.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const childModeEnv = "NSELF_TEST_DETACH_CHILD"

// TestMain turns the test binary into the detached child when asked to.
func TestMain(m *testing.M) {
	if mode := os.Getenv(childModeEnv); mode != "" {
		fakeDetachedChild(mode)
		return
	}
	os.Exit(m.Run())
}

// fakeDetachedChild writes what a real child would: stderr text, then the queued record.
func fakeDetachedChild(mode string) {
	switch mode {
	case "exit":
		fmt.Fprintln(os.Stderr, "fatal: bad config, credential "+fakeToken())
		os.Exit(3)
	case "spoof": // a well-formed queued line for a run that does not exist, then exit
		fmt.Fprintln(os.Stderr, evLine("queued", "r-spoofed", 1))
		os.Exit(4)
	case "queued":
		id := os.Getenv("NSELF_TEST_RUN_ID")
		st, err := store.Open(filepath.Join(os.Getenv("NSELF_CI_HOME"), "state.db"), store.Options{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
		_ = st.CreatePipeline(context.Background(), store.PipelineRow{ID: id, Revision: "rev", Project: "p", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
		_ = st.Close()
		fmt.Fprintln(os.Stderr, "argv: "+strings.Join(os.Args[1:], " "))
		tok, ok := os.LookupEnv("NSELF_MCP_TOKEN")
		fmt.Fprintf(os.Stderr, "mcp: set=%v\n", ok && tok != "")
		fmt.Println(evLine("queued", id, 1))
		time.Sleep(200 * time.Millisecond)
	}
}

// fakeToken is a GitHub-token-shaped fake, built so no scanner sees a literal.
func fakeToken() string { return "ghp_" + strings.Repeat("a1B2", 9) }

type hookExit int

// detachRun parses args as bare `nself ci` would and returns what the hook printed
// and the exit it asked for (-1 when it did not exit).
func detachRun(t *testing.T, args []string) (code int, out string) {
	t.Helper()
	var buf bytes.Buffer
	oldOut, oldExit, oldArgs, oldTune := detachOut, detachExit, detachArgs, detachTune
	detachOut, detachArgs = &buf, func() []string { return args }
	detachExit = func(c int) { panic(hookExit(c)) }
	t.Cleanup(func() { detachOut, detachExit, detachArgs, detachTune = oldOut, oldExit, oldArgs, oldTune })
	code = -1
	func() {
		defer func() {
			if r := recover(); r != nil {
				h, ok := r.(hookExit)
				if !ok {
					panic(r)
				}
				code = int(h)
			}
		}()
		_, _ = parseBare(args)
	}()
	return code, buf.String()
}

type handleOut struct {
	Data    detach.Handle
	ErrBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeHandle(t *testing.T, out string) handleOut {
	t.Helper()
	var h handleOut
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, out)
	}
	return h
}

func detachEnv(t *testing.T, mode, runID string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	t.Setenv(childModeEnv, mode)
	t.Setenv("NSELF_TEST_RUN_ID", runID)
	return home
}

func TestDetach(t *testing.T) {
	t.Run("returns the run handle in under 5 s and the parent output is one envelope", func(t *testing.T) {
		home := detachEnv(t, "queued", "r-det-1")
		start := time.Now()
		code, out := detachRun(t, []string{"--detach", "--json", "--no-status"})
		h := decodeHandle(t, out).Data
		if code != 0 || time.Since(start) > 5*time.Second {
			t.Fatalf("exit %d in %v\n%s", code, time.Since(start), out)
		}
		if h.Schema != "ci.run-handle/v1" || h.RunID != "r-det-1" || h.State != "queued" || h.PID <= 0 || h.Coordinator != "local" {
			t.Fatalf("handle %+v", h)
		}
		for name, argv := range map[string][]string{"wait": h.Commands.Wait, "follow": h.Commands.Follow, "evidence": h.Commands.Evidence, "cancel": h.Commands.Cancel, "logs": h.Commands.Logs} {
			if len(argv) < 3 || argv[0] != "nself" || argv[1] != "ci" || !strings.Contains(strings.Join(argv, " "), "r-det-1") {
				t.Errorf("%s argv %v", name, argv)
			}
		}
		if want := filepath.Join(home, "detached", "r-det-1.ndjson"); h.StreamPath != want {
			t.Fatalf("stream path %q, want %q", h.StreamPath, want)
		}
		if st, err := os.Stat(h.StreamPath); err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("stream file: %v %v", st, err)
		}
		if st, err := os.Stat(filepath.Dir(h.StreamPath)); err != nil || st.Mode().Perm() != 0o700 {
			t.Fatalf("stream dir: %v %v", st, err)
		}
	})
	t.Run("the child gets the argv with --json --stream and no NSELF_MCP_TOKEN", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-2")
		t.Setenv("NSELF_MCP_TOKEN", "fake-mcp-token-value")
		code, out := detachRun(t, []string{"--detach", "--job", "build", "--json"})
		h := decodeHandle(t, out).Data
		b, err := os.ReadFile(h.StreamPath)
		if code != 0 || err != nil {
			t.Fatalf("exit %d %v\n%s", code, err, out)
		}
		if !strings.Contains(string(b), "argv: --json --stream --job build\n") || strings.Contains(string(b), "--detach") {
			t.Fatalf("child argv wrong:\n%s", b)
		}
		if !strings.Contains(string(b), "mcp: set=false") || strings.Contains(string(b), "fake-mcp-token-value") {
			t.Fatalf("child saw the machine token:\n%s", b)
		}
	})
	t.Run("detach with an ad-hoc command keeps the command after --", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-3")
		code, out := detachRun(t, []string{"--detach", "--json", "--", "go", "test", "./..."})
		h := decodeHandle(t, out).Data
		b, _ := os.ReadFile(h.StreamPath)
		if code != 0 || !strings.Contains(string(b), "argv: --json --stream -- go test ./...\n") {
			t.Fatalf("exit %d\n%s", code, b)
		}
	})
	t.Run("detach with --coordinator puts it on the wait and follow commands", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-4")
		if bareFlags.Lookup("coordinator") == nil { // registered by P7-NODE's cmd/v15_coordinator.go once it lands
			bareFlags.String("coordinator", "", "coordinator URL (test stand-in)")
		}
		const url = "http://127.0.0.1:18443"
		code, out := detachRun(t, []string{"--detach", "--json", "--coordinator", url})
		h := decodeHandle(t, out).Data
		join := func(a []string) string { return strings.Join(a, " ") }
		if code != 0 || h.Coordinator != url || !strings.HasSuffix(join(h.Commands.Wait), "--coordinator "+url) || !strings.HasSuffix(join(h.Commands.Follow), "--coordinator "+url) {
			t.Fatalf("exit %d handle %+v", code, h)
		}
	})
	t.Run("a child that exits before queued is E648 child_exit with a redacted tail", func(t *testing.T) {
		detachEnv(t, "exit", "")
		code, out := detachRun(t, []string{"--detach", "--json"})
		e := decodeHandle(t, out).ErrBody
		if code != 2 || e.Code != "E648" || !strings.Contains(e.Message, "child_exit") || !strings.Contains(e.Message, "bad config") {
			t.Fatalf("exit %d %+v\n%s", code, e, out)
		}
		if strings.Contains(out, fakeToken()) {
			t.Fatalf("the token shape leaked into the error:\n%s", out)
		}
	})
	t.Run("a queued line that stderr printed for a run that does not exist is not believed", func(t *testing.T) {
		detachEnv(t, "spoof", "")
		code, out := detachRun(t, []string{"--detach", "--json"})
		e := decodeHandle(t, out).ErrBody
		if code != 2 || e.Code != "E648" || !strings.Contains(e.Message, "child_exit") {
			t.Fatalf("exit %d %+v\n%s", code, e, out)
		}
	})
	t.Run("a repository argument does not hide --json --stream from the child", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-8")
		repo := t.TempDir()
		code, out := detachRun(t, []string{"--detach", "--json", repo})
		b, _ := os.ReadFile(decodeHandle(t, out).Data.StreamPath)
		if code != 0 || !strings.Contains(string(b), "argv: --json --stream "+repo+"\n") {
			t.Fatalf("exit %d\n%s", code, b)
		}
	})
	t.Run("human output prints the run and its commands", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-5")
		code, out := detachRun(t, []string{"--detach"})
		if code != 0 || !strings.HasPrefix(out, "run r-det-5 queued (pid ") || !strings.Contains(out, "nself ci runs wait r-det-5\n") {
			t.Fatalf("exit %d\n%s", code, out)
		}
	})
	t.Run("a new detach sweeps the streams of runs that retention pruned", func(t *testing.T) {
		home := detachEnv(t, "queued", "r-det-7")
		seedRun(t, home, "r-keep", "passed", time.Now(), nil)
		old := time.Now().Add(-3 * time.Hour)
		for _, id := range []string{"r-keep", "r-pruned"} {
			p := streamFile(t, home, id, evLine("queued", id, 1))
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		if code, out := detachRun(t, []string{"--detach", "--json"}); code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if _, err := os.Stat(filepath.Join(detach.Dir(home), "r-keep.ndjson")); err != nil {
			t.Fatalf("the stream of a kept run was removed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(detach.Dir(home), "r-pruned.ndjson")); !os.IsNotExist(err) {
			t.Fatalf("the stream of a pruned run survived: %v", err)
		}
	})
	t.Run("without --detach the hook does nothing", func(t *testing.T) {
		detachEnv(t, "queued", "r-det-6")
		if code, out := detachRun(t, []string{"--json"}); code != -1 || out != "" {
			t.Fatalf("exit %d out %q", code, out)
		}
	})
	t.Run("--detach is a bare flag of the V15 set only", func(t *testing.T) {
		if bareFlags.Lookup("detach") == nil {
			t.Fatal("--detach is not registered")
		}
		if !strings.Contains(v15Help(), "-detach") {
			t.Fatal("--detach is missing from help")
		}
	})
}

// TestDetachPrune: `runs prune` removes the stream files of the runs it pruned.
func TestDetachPrune(t *testing.T) {
	home := runsHome(t)
	now := time.Now()
	for i, id := range []string{"r-old", "r-mid", "r-new"} {
		seedRun(t, home, id, "passed", now.Add(time.Duration(i-5)*time.Hour), map[string]string{"build": "passed"})
		streamFile(t, home, id, evLine("queued", id, 1), evLine("result", id, 2))
	}
	if code, _, errs := capture(t, func() int { return runsSubs["prune"]([]string{"--keep", "1", "--dry-run", "--json"}) }); code != 0 {
		t.Fatalf("dry run exit %d %s", code, errs)
	}
	for _, id := range []string{"r-old", "r-mid", "r-new"} {
		if _, err := os.Stat(filepath.Join(detach.Dir(home), id+".ndjson")); err != nil {
			t.Fatalf("a dry run removed %s: %v", id, err)
		}
	}
	code, out, errs := capture(t, func() int { return runsSubs["prune"]([]string{"--keep", "1", "--json"}) })
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, out, errs)
	}
	for id, want := range map[string]bool{"r-old": false, "r-mid": false, "r-new": true} {
		_, err := os.Stat(filepath.Join(detach.Dir(home), id+".ndjson"))
		if (err == nil) != want {
			t.Errorf("%s stream kept=%v, want %v (%v)", id, err == nil, want, err)
		}
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("prune output is no longer one document:\n%s", out)
	}
}

func TestDetachDryRunFlagForms(t *testing.T) {
	for args, want := range map[string]bool{"--dry-run": true, "-dry-run=true": true, "--dry-run=1": true, "--dry-run=t": true, "--dry-run=false": false, "--keep": false, "x": false} {
		if got := hasDryRun([]string{args}); got != want {
			t.Errorf("hasDryRun(%q) = %v, want %v", args, got, want)
		}
	}
}
