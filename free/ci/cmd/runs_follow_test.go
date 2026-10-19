package main

// Tests of `nself ci runs follow` (P7-CI-63): replay of a detached run's event
// file from a seq, tailing a file being written, the rebuilt stream of a run
// without a file, a run that died without a result, and unknown runs.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
)

func evLine(typ, run string, seq int) string {
	return fmt.Sprintf(`{"schema_version":"1","command":"ci","data":{"type":%q,"schema":"ci.events/v1","seq":%d,"ts":"2026-10-19T00:00:00Z","run_id":%q,"pipeline_id":%q,"data":{}}}`, typ, seq, run, run)
}

func streamFile(t *testing.T, home, run string, lines ...string) string {
	t.Helper()
	dir := detach.Dir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, run+".ndjson")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, l := range lines {
		if _, err = f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// parsed returns the type and seq of every line of out; every line must be a record or an error.
func parsed(t *testing.T, out string) (types []string, seqs []int64) {
	t.Helper()
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		r, ok := detach.ParseLine([]byte(l))
		if !ok {
			t.Fatalf("not a stream line: %q", l)
		}
		if r.Err {
			types = append(types, "error")
			continue
		}
		types, seqs = append(types, r.Type), append(seqs, r.Seq)
	}
	return
}

func increasing(seqs []int64) bool {
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			return false
		}
	}
	return true
}

func TestRunsFollow(t *testing.T) {
	t.Run("replays a detached run from a seq and ends with the evidence", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-file", "passed", time.Now(), map[string]string{"build": "passed"})
		streamFile(t, home, "r-file", evLine("queued", "r-file", 1), "warning: stderr text", evLine("planned", "r-file", 2),
			evLine("started", "r-file", 3), evLine("check_result", "r-file", 4), evLine("result", "r-file", 5))
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-file", "--from-seq", "3", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || strings.Join(types, ",") != "started,check_result,result" || !increasing(seqs) || strings.Contains(out, "stderr text") {
			t.Fatalf("exit %d types %v seqs %v\n%s", code, types, seqs, out)
		}
		code, out, _ = capture(t, func() int { return runFollow([]string{"r-file", "--from-seq", "0"}) })
		if types, _ = parsed(t, out); code != 0 || types[0] != "queued" || types[len(types)-1] != "result" || len(types) != 5 {
			t.Fatalf("full replay: exit %d %v", code, types)
		}
	})
	t.Run("started mid-run it replays then tails", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-tail", "running", time.Now(), map[string]string{"build": "running"})
		path := streamFile(t, home, "r-tail", evLine("queued", "r-tail", 1), evLine("started", "r-tail", 2))
		go func() {
			for _, l := range []string{evLine("check_result", "r-tail", 3), evLine("finished", "r-tail", 4), evLine("result", "r-tail", 5)} {
				time.Sleep(60 * time.Millisecond)
				f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
				_, _ = f.WriteString(l + "\n")
				_ = f.Close()
			}
			finishRun(t, home, "r-tail", "passed")
		}()
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-tail", "--from-seq", "0", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || len(types) != 5 || types[4] != "result" || !increasing(seqs) {
			t.Fatalf("exit %d %v %v", code, types, seqs)
		}
	})
	t.Run("a run without an event file gets a rebuilt stream", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-nofile", "failed", time.Now(), map[string]string{"build": "passed", "test": "failed"})
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-nofile", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || types[0] != "queued" || types[len(types)-2] != "finished" || types[len(types)-1] != "result" || !increasing(seqs) || seqs[0] != 1 {
			t.Fatalf("exit %d %v %v", code, types, seqs)
		}
		if !strings.Contains(out, `"run_id":"r-nofile"`) || strings.Count(out, `"type":"check_result"`) != 2 {
			t.Fatalf("stream:\n%s", out)
		}
	})
	t.Run("a run that died without a result ends with an E648 error line", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-lost", "running", time.Now().Add(-10*time.Minute), nil)
		streamFile(t, home, "r-lost", evLine("queued", "r-lost", 1), evLine("started", "r-lost", 2))
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-lost", "--json"}) })
		types, _ := parsed(t, out)
		if code != 2 || types[len(types)-1] != "error" || !strings.Contains(out, `"code":"E648"`) || !strings.Contains(out, "child_exit") {
			t.Fatalf("exit %d %v\n%s", code, types, out)
		}
	})
	t.Run("a finished run whose file lost its last line is closed with a result", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-cut", "passed", time.Now(), map[string]string{"build": "passed"})
		streamFile(t, home, "r-cut", evLine("queued", "r-cut", 1), evLine("started", "r-cut", 2))
		code, out, _ := capture(t, func() int { return runFollow([]string{"r-cut", "--json"}) })
		types, seqs := parsed(t, out)
		if code != 0 || types[len(types)-1] != "result" || !increasing(seqs) || seqs[len(seqs)-1] != 3 {
			t.Fatalf("exit %d %v %v\n%s", code, types, seqs, out)
		}
	})
	t.Run("unknown run and bad usage", func(t *testing.T) {
		runsHome(t)
		code, out, _ := capture(t, func() int { return runFollow([]string{"nope", "--json"}) })
		if code != 1 || !strings.Contains(out, `"code":"E626"`) {
			t.Fatalf("exit %d\n%s", code, out)
		}
		for _, args := range [][]string{nil, {"a", "b"}, {"a", "--from-seq", "-1"}} {
			if code, _, _ = capture(t, func() int { return runFollow(args) }); code != 1 {
				t.Fatalf("%v: exit %d", args, code)
			}
		}
	})
}
