package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type line struct {
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
}

func lines(t *testing.T, b []byte) []line {
	t.Helper()
	var out []line
	for _, raw := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if raw == "" {
			continue
		}
		if err := output.CheckEnvelope([]byte(raw)); err != nil {
			t.Fatalf("not a v1 envelope: %v\n%s", err, raw)
		}
		var l line
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func fixedClock() func() time.Time {
	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return func() time.Time { t0 = t0.Add(time.Second); return t0 }
}

func TestEmitterSequenceAndFraming(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, "ci", "run-1", "run-1", fixedClock())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.Emit("log", "job", "", map[string]any{"bytes": 3}) }()
	}
	wg.Wait()
	e.Emit("queued", "", "", nil)
	if seq := e.End(map[string]any{"result": "pass"}, nil); seq != 22 {
		t.Fatalf("end seq %d", seq)
	}
	if e.Emit("log", "", "", nil) != 0 || e.End(nil, nil) != 0 {
		t.Fatal("a closed stream kept writing")
	}
	all := lines(t, buf.Bytes())
	if len(all) != 22 {
		t.Fatalf("%d lines", len(all))
	}
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	var prev int64
	for i, l := range all {
		var d struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		if err = json.Unmarshal(l.Data, &d); err != nil || d.Seq != prev+1 {
			t.Fatalf("line %d: seq %d after %d (%v)", i, d.Seq, prev, err)
		}
		prev = d.Seq
		if i < 21 {
			if err = model.ValidateJSON(schemas["ci-events.v1.schema.json"], l.Data); err != nil {
				t.Fatalf("line %d is not a ci.events/v1 record: %v\n%s", i, err, l.Data)
			}
		} else if d.Type != output.ResultType {
			t.Fatalf("last line type %q", d.Type)
		}
	}
	if !strings.Contains(string(all[20].Data), `"type":"queued"`) {
		t.Fatalf("queued record: %s", all[20].Data)
	}
}

func TestEmitterRefusesWhatItCannotFrame(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, "ci", "r", "r", nil)
	if e.Emit("not-a-type", "", "", nil) != 0 || !e.Flush() || buf.Len() != 0 {
		t.Fatal("an unknown event type was written")
	}
	if e.Emit("evidence", "", "", nil) == 0 {
		t.Fatal("a valid type was refused")
	}
	e.Flush()
}

type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n--; f.n < 0 {
		return 0, errors.New("broken pipe")
	}
	return len(p), nil
}

func TestEmitterStopsAfterAWriteError(t *testing.T) {
	w := &failWriter{n: 1}
	e := New(w, "ci", "r", "r", nil)
	if e.Emit("queued", "", "", nil) != 1 || !e.Flush() || e.Err() != nil {
		t.Fatal("first write")
	}
	e.Emit("planned", "", "", nil)
	e.Flush()
	if e.Err() == nil {
		t.Fatal("a failed write must be reported")
	}
	if e.Emit("started", "", "", nil) != 0 || e.End(nil, nil) != 0 {
		t.Fatal("the emitter wrote after a failure")
	}
}

func TestEndCarriesMetaAndTheResult(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, "ci evidence verify", "r", "r", nil)
	e.End(map[string]any{"revision": "abc"}, &output.Meta{Warnings: []string{"note"}})
	if got := buf.String(); !strings.Contains(got, `"command":"ci evidence verify"`) || !strings.Contains(got, `"warnings":["note"]`) ||
		!strings.Contains(got, `"evidence":{"revision":"abc"}`) || !strings.Contains(got, `"type":"result"`) {
		t.Fatalf("end line: %s", got)
	}
}

// blockedWriter stalls every write until release is closed and counts lines.
type blockedWriter struct {
	release chan struct{}
	mu      *sync.Mutex
	buf     *bytes.Buffer
}

func (b blockedWriter) Write(p []byte) (int, error) {
	<-b.release
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// A consumer that keeps its pipe open but stops reading must not block the
// emitter's callers (the job output readers and the coordinator): every Emit and
// TryEmit returns at once, best-effort records are dropped once a backlog
// builds, and nothing that mattered is lost or reordered when the consumer
// resumes.
func TestStalledConsumerNeverBlocksTheCallers(t *testing.T) {
	w := blockedWriter{release: make(chan struct{}), mu: &sync.Mutex{}, buf: &bytes.Buffer{}}
	e := New(w, "ci", "r", "r", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Emit("queued", "", "", nil)
		for i := 0; i < 5*maxLogBacklog; i++ {
			e.TryEmit("log", "job", "", map[string]any{"bytes": 1})
		}
		e.Emit("started", "job", "", nil)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("an Emit waited for the stalled consumer")
	}
	close(w.release)
	if seq := e.End(map[string]any{"result": "pass"}, nil); seq == 0 {
		t.Fatal("End failed after the consumer resumed")
	}
	w.mu.Lock()
	out := w.buf.Bytes()
	w.mu.Unlock()
	all := lines(t, out)
	var prev int64
	logs, started := 0, false
	var order []string
	for i, l := range all {
		var d struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		if err := json.Unmarshal(l.Data, &d); err != nil || d.Seq != prev+1 {
			t.Fatalf("line %d: seq %d after %d (a dropped record must leave no gap): %v", i, d.Seq, prev, err)
		}
		prev = d.Seq
		logs += map[bool]int{true: 1}[d.Type == "log"]
		started = started || d.Type == "started"
		if len(order) == 0 || order[len(order)-1] != d.Type {
			order = append(order, d.Type)
		}
	}
	if strings.Join(order, ",") != "queued,log,started,result" {
		t.Fatalf("record order %v: queue order must survive the drops", order)
	}
	if !started || logs == 0 || logs >= 5*maxLogBacklog {
		t.Fatalf("started=%v, %d of %d log records kept: want the started record and a bounded number of logs", started, logs, 5*maxLogBacklog)
	}
}

// A consumer that never reads again cannot hold End for ever.
func TestEndGivesUpOnAStalledConsumer(t *testing.T) {
	old := FlushTimeout
	FlushTimeout = 200 * time.Millisecond
	defer func() { FlushTimeout = old }()
	w := blockedWriter{release: make(chan struct{}), mu: &sync.Mutex{}, buf: &bytes.Buffer{}}
	defer close(w.release)
	e := New(w, "ci", "r", "r", nil)
	if seq := e.End(map[string]any{"result": "pass"}, nil); seq != 0 || e.Err() == nil {
		t.Fatalf("End returned %d with err %v for a consumer that never reads", seq, e.Err())
	}
}

// After End gave up, Flush must not wait out a second timeout.
func TestFlushAfterAStalledEndReturnsAtOnce(t *testing.T) {
	old := FlushTimeout
	FlushTimeout = 200 * time.Millisecond
	defer func() { FlushTimeout = old }()
	w := blockedWriter{release: make(chan struct{}), mu: &sync.Mutex{}, buf: &bytes.Buffer{}}
	defer close(w.release)
	e := New(w, "ci", "r", "r", nil)
	e.End(map[string]any{"result": "pass"}, nil)
	start := time.Now()
	if e.Flush() || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("Flush after a stalled End waited %v", time.Since(start))
	}
}
