package events

// Purpose: the ci.events/v1 stream (Epic D3, contract:cli.json-stream v1):
// one compact v1 envelope per line, data.type naming the record, seq strictly
// increasing from 1 per run, the last line data.type "result".
// Inputs:  event types with their job, attempt and data.
// Outputs: NDJSON lines on a writer; the framing comes from sdk/go/output.
// Constraints: events carry no log text (a `log` record holds a byte count and
// no content, until the redactor owns log refs); one mutex orders seq and
// writes together so a line never overtakes a lower seq; after the first write
// error the emitter stops writing and reports it through Err; Emit never
// panics and a consumer's closed pipe never fails the run.

import (
	"io"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Schema is the contract id every record carries.
const Schema = "ci.events/v1"

// Emitter writes the records of one run.
type Emitter struct {
	mu          sync.Mutex
	w           io.Writer
	command     string
	runID, pipe string
	now         func() time.Time
	seq         int64
	err         error
	ended       bool
}

// New makes an emitter for one run. command is the envelope's command name; a
// nil clock uses the wall clock.
func New(w io.Writer, command, runID, pipelineID string, now func() time.Time) *Emitter {
	if now == nil {
		now = time.Now
	}
	return &Emitter{w: w, command: command, runID: runID, pipe: pipelineID, now: now}
}

// Emit writes one record and returns its seq (0 when nothing was written).
func (e *Emitter) Emit(typ model.EventType, job, attempt string, data map[string]any) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil || e.ended || !typ.Valid() {
		return 0
	}
	seq := e.seq + 1
	line, err := output.StreamRecord(e.command, string(typ), e.fields(seq, job, attempt, data)...)
	if err != nil {
		e.err = err
		return 0
	}
	if _, err = e.w.Write(line); err != nil {
		e.err = err
		return 0
	}
	e.seq = seq
	return seq
}

// End writes the last line: data.type "result" holding the evidence event.
// result is the document the consumer reads (the evidence, or a verify report);
// meta may be nil.
func (e *Emitter) End(result any, meta *output.Meta) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil || e.ended {
		return 0
	}
	seq := e.seq + 1
	fields := append(e.fields(seq, "", "", nil), output.Field{Key: "event", Value: string(model.EventType("evidence"))}, output.Field{Key: "evidence", Value: result})
	fields = dropKey(fields, "data")
	line, err := output.StreamEnd(e.command, meta, fields...)
	if err != nil {
		e.err = err
		return 0
	}
	if _, err = e.w.Write(line); err != nil {
		e.err = err
		return 0
	}
	e.seq, e.ended = seq, true
	return seq
}

// Err is the first error the emitter met, nil when every line was written.
func (e *Emitter) Err() error { e.mu.Lock(); defer e.mu.Unlock(); return e.err }

func (e *Emitter) fields(seq int64, job, attempt string, data map[string]any) []output.Field {
	if data == nil {
		data = map[string]any{}
	}
	f := []output.Field{
		{Key: "schema", Value: Schema}, {Key: "seq", Value: seq}, {Key: "ts", Value: e.now().UTC().Format(time.RFC3339Nano)},
		{Key: "run_id", Value: e.runID}, {Key: "pipeline_id", Value: e.pipe},
	}
	if job != "" {
		f = append(f, output.Field{Key: "job_id", Value: job})
	}
	if attempt != "" {
		f = append(f, output.Field{Key: "attempt_id", Value: attempt})
	}
	return append(f, output.Field{Key: "data", Value: data})
}

func dropKey(f []output.Field, key string) []output.Field {
	out := f[:0]
	for _, x := range f {
		if x.Key != key {
			out = append(out, x)
		}
	}
	return out
}
