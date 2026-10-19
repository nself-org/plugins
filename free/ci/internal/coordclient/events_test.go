package coordclient

// Purpose: tests of the event stream client: reconnect by seq with no duplicate and no missing record,
// refusal without retry, and the bound on a stream that never delivers.
// Inputs:  an httptest server that cuts, replays or skips records on purpose.
// Outputs: pass or fail.
// Constraints: plain HTTP on 127.0.0.1 (the TLS client path has its own tests); reconnect waits shortened.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
)

// runLines is the record stream of a finished two-job run, one line each.
func runLines(t *testing.T) []string {
	t.Helper()
	var buf bytes.Buffer
	s := detach.NewSynth(&buf, "run-1", 0)
	s.Observe(detach.View{RunID: "run-1", State: "running"})
	s.Finish(detach.View{RunID: "run-1", State: "passed", Jobs: []detach.JobState{{Name: "build", State: "passed"}, {Name: "test", State: "passed"}}}, map[string]any{"result": "pass"})
	s.Flush()
	return strings.Split(strings.TrimSpace(buf.String()), "\n")
}

func seqOf(t *testing.T, line string) int64 {
	t.Helper()
	rec, ok := detach.ParseLine([]byte(line))
	if !ok {
		t.Fatalf("not a record: %q", line)
	}
	return rec.Seq
}

// flaky serves the records; plan(n, from) says how many lines request n sends and which to skip.
type flaky struct {
	t     *testing.T
	lines []string
	mu    sync.Mutex
	reqs  []int64 // the from= of every request
	plan  func(n int, from int64) (send []int, cut bool)
}

func (f *flaky) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	f.mu.Lock()
	n := len(f.reqs)
	f.reqs = append(f.reqs, from)
	f.mu.Unlock()
	send, cut := f.plan(n, from)
	for _, i := range send {
		_, _ = w.Write([]byte(f.lines[i] + "\n"))
		w.(http.Flusher).Flush()
	}
	if cut {
		panic(http.ErrAbortHandler) // the connection drops mid-stream
	}
}

func drive(t *testing.T, f *flaky) ([]int64, error) {
	t.Helper()
	srv := httptest.NewServer(f)
	defer srv.Close()
	old := Reconnect
	Reconnect.First, Reconnect.Longest, Reconnect.MaxIdle = time.Millisecond, 4*time.Millisecond, 4
	defer func() { Reconnect = old }()
	c := &Client{base: srv.URL, hc: srv.Client()}
	var got []int64
	_, err := c.Events(context.Background(), "run-1", func(line []byte, rec detach.Record) error {
		got = append(got, rec.Seq)
		return nil
	})
	return got, err
}

func want(n int) []int64 {
	var out []int64
	for i := 1; i <= n; i++ {
		out = append(out, int64(i))
	}
	return out
}

func TestCoordinatorClientEventsReconnectBySeq(t *testing.T) {
	lines := runLines(t)
	total := len(lines)
	t.Run("cut mid-stream resumes at the next seq", func(t *testing.T) {
		f := &flaky{t: t, lines: lines}
		f.plan = func(n int, from int64) ([]int, bool) {
			if n == 0 {
				return []int{0, 1, 2}, true
			}
			var all []int
			for i := int(from) - 1; i < total; i++ {
				all = append(all, i)
			}
			return all, false
		}
		got, err := drive(t, f)
		if err != nil || len(got) != total || !equal(got, want(total)) {
			t.Fatalf("got %v err %v, want 1..%d once each", got, err, total)
		}
		if len(f.reqs) != 2 || f.reqs[0] != 1 || f.reqs[1] != 4 {
			t.Fatalf("requests asked from %v, want [1 4]", f.reqs)
		}
	})
	t.Run("a server that replays from the start causes no duplicate", func(t *testing.T) {
		f := &flaky{t: t, lines: lines}
		f.plan = func(n int, _ int64) ([]int, bool) {
			if n == 0 {
				return []int{0, 1}, true
			}
			all := make([]int, total)
			for i := range all {
				all[i] = i
			}
			return all, false
		}
		got, err := drive(t, f)
		if err != nil || !equal(got, want(total)) {
			t.Fatalf("got %v err %v", got, err)
		}
	})
	t.Run("a skipped record is never passed over", func(t *testing.T) {
		f := &flaky{t: t, lines: lines}
		f.plan = func(n int, from int64) ([]int, bool) {
			if n == 0 {
				return []int{0, 2, 3}, true // record 2 is missing
			}
			var all []int
			for i := int(from) - 1; i < total; i++ {
				all = append(all, i)
			}
			return all, false
		}
		got, err := drive(t, f)
		if err != nil || !equal(got, want(total)) {
			t.Fatalf("got %v err %v, want an unbroken 1..%d", got, err, total)
		}
		if f.reqs[1] != 2 {
			t.Fatalf("the retry asked from %d, want 2", f.reqs[1])
		}
	})
	t.Run("a result that jumps ahead is a gap", func(t *testing.T) {
		f := &flaky{t: t, lines: lines}
		f.plan = func(n int, from int64) ([]int, bool) {
			if n == 0 {
				return []int{0, total - 1}, false // queued, then the result with everything between missing
			}
			var all []int
			for i := int(from) - 1; i < total; i++ {
				all = append(all, i)
			}
			return all, false
		}
		got, err := drive(t, f)
		if err != nil || !equal(got, want(total)) {
			t.Fatalf("got %v err %v", got, err)
		}
	})
	t.Run("a stream that never delivers stops", func(t *testing.T) {
		f := &flaky{t: t, lines: lines}
		f.plan = func(int, int64) ([]int, bool) { return nil, true }
		_, err := drive(t, f)
		if !reasonIs(err, "transport") || len(f.reqs) != 4 {
			t.Fatalf("err %v after %d requests, want transport after 4", err, len(f.reqs))
		}
	})
}

func TestCoordinatorClientEventsRefusalIsNotRetried(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"code":"E664","reason":"role","message":"the caller may not do this"}`))
	}))
	defer srv.Close()
	c := &Client{base: srv.URL, hc: srv.Client()}
	_, err := c.Events(context.Background(), "run-1", func([]byte, detach.Record) error { return nil })
	if !reasonIs(err, "role") || n != 1 {
		t.Fatalf("err %v after %d requests, want role after 1", err, n)
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
