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
	if e.Emit("not-a-type", "", "", nil) != 0 || buf.Len() != 0 {
		t.Fatal("an unknown event type was written")
	}
	if e.Emit("evidence", "", "", nil) == 0 {
		t.Fatal("a valid type was refused")
	}
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
	if e.Emit("queued", "", "", nil) != 1 || e.Err() != nil {
		t.Fatal("first write")
	}
	if e.Emit("planned", "", "", nil) != 0 || e.Err() == nil {
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

// blockedWriter stalls every write until release is closed.
type blockedWriter struct{ release chan struct{} }

func (b blockedWriter) Write(p []byte) (int, error) { <-b.release; return len(p), nil }

// A consumer that stops reading must not hold up best-effort records: TryEmit
// returns at once while another record is stuck in a write.
func TestTryEmitNeverWaitsBehindAStalledConsumer(t *testing.T) {
	w := blockedWriter{release: make(chan struct{})}
	e := New(w, "ci", "r", "r", nil)
	stuck := make(chan int64)
	go func() { stuck <- e.Emit("queued", "", "", nil) }()
	for e.mu.TryLock() { // wait until the writer goroutine holds the lock
		e.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	done := make(chan int64)
	go func() { done <- e.TryEmit("log", "job", "", nil) }()
	select {
	case seq := <-done:
		if seq != 0 {
			t.Fatalf("a record was written behind a stalled write: seq %d", seq)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TryEmit waited for the stalled consumer")
	}
	close(w.release)
	if seq := <-stuck; seq != 1 {
		t.Fatalf("the stalled record got seq %d", seq)
	}
	if seq := e.TryEmit("log", "job", "", nil); seq != 2 {
		t.Fatalf("seq %d after the consumer resumed, want 2 (a dropped record leaves no gap)", seq)
	}
}
