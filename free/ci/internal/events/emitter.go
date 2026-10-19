package events

// Purpose: the ci.events/v1 stream (Epic D3, contract:cli.json-stream v1):
// one compact v1 envelope per line, data.type naming the record, seq strictly
// increasing from 1 per run, the last line data.type "result".
// Inputs:  event types with their job, attempt and data.
// Outputs: NDJSON lines on a writer; the framing comes from sdk/go/output.
// Constraints: events carry no log text (a `log` record holds a byte count and
// no content, until the redactor owns log refs); records are queued and one
// goroutine writes them, so a consumer that stops reading can never block a
// job's output reader or the coordinator (best-effort `log` records are
// dropped once a backlog builds, with no gap in seq; every other record is
// kept); seq is assigned under one mutex in queue order; after the first write
// error the emitter stops writing and reports it through Err; Emit never
// panics and a consumer's closed pipe never fails the run; End waits at most
// FlushTimeout for the consumer.

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Schema is the contract id every record carries.
const Schema = "ci.events/v1"

// FlushTimeout bounds how long End and Flush wait for a slow consumer.
var FlushTimeout = 10 * time.Second

// maxLogBacklog is how many best-effort log records may wait for the writer.
const maxLogBacklog = 256

// Emitter writes the records of one run.
type Emitter struct {
	mu          sync.Mutex
	cond        *sync.Cond
	w           io.Writer
	command     string
	runID, pipe string
	now         func() time.Time
	seq         int64
	err         error
	ended       bool
	closed      bool // the writer goroutine has finished
	started     bool
	queue       []item
	logs        int // log records waiting in the queue
}

// item is one queued line, or a flush marker (line nil).
type item struct {
	line  []byte
	log   bool
	flush chan struct{}
	last  bool
}

// New makes an emitter for one run. command is the envelope's command name; a
// nil clock uses the wall clock.
func New(w io.Writer, command, runID, pipelineID string, now func() time.Time) *Emitter {
	if now == nil {
		now = time.Now
	}
	e := &Emitter{w: w, command: command, runID: runID, pipe: pipelineID, now: now}
	e.cond = sync.NewCond(&e.mu)
	return e
}

// Emit queues one record and returns its seq (0 when nothing was queued).
func (e *Emitter) Emit(typ model.EventType, job, attempt string, data map[string]any) int64 {
	return e.enqueue(typ, job, attempt, data, false)
}

// TryEmit is Emit for best-effort records (log byte counts): when a backlog
// has built up behind a slow consumer it drops the record and returns 0.
func (e *Emitter) TryEmit(typ model.EventType, job, attempt string, data map[string]any) int64 {
	return e.enqueue(typ, job, attempt, data, true)
}

func (e *Emitter) enqueue(typ model.EventType, job, attempt string, data map[string]any, bestEffort bool) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil || e.ended || !typ.Valid() || (bestEffort && e.logs >= maxLogBacklog) {
		return 0
	}
	seq := e.seq + 1
	line, err := output.StreamRecord(e.command, string(typ), e.fields(seq, job, attempt, data)...)
	if err != nil {
		e.err = err
		return 0
	}
	e.seq = seq
	e.push(item{line: line, log: bestEffort})
	return seq
}

// push adds it to the queue and starts the writer on first use. mu is held.
func (e *Emitter) push(it item) {
	if it.log {
		e.logs++
	}
	e.queue = append(e.queue, it)
	if !e.started {
		e.started = true
		go e.write()
	}
	e.cond.Signal()
}

// write is the only goroutine that touches the writer.
func (e *Emitter) write() {
	for {
		e.mu.Lock()
		for len(e.queue) == 0 {
			e.cond.Wait()
		}
		batch := e.queue
		e.queue = nil
		e.mu.Unlock()
		for i, it := range batch {
			if it.line != nil {
				e.mu.Lock()
				dead := e.err != nil
				e.mu.Unlock()
				var err error
				if !dead {
					_, err = e.w.Write(it.line)
				}
				e.mu.Lock()
				if err != nil && e.err == nil {
					e.err = err
				}
				if it.log {
					e.logs--
				}
				e.mu.Unlock()
			}
			if it.flush != nil {
				close(it.flush)
			}
			if it.last {
				e.mu.Lock()
				e.closed = true
				rest := append(batch[i+1:], e.queue...)
				e.queue = nil
				e.mu.Unlock()
				for _, r := range rest {
					if r.flush != nil {
						close(r.flush)
					}
				}
				return
			}
		}
	}
}

// Flush waits until everything queued so far was written (or FlushTimeout
// passed) and reports whether it was.
func (e *Emitter) Flush() bool {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return true
	}
	done := make(chan struct{})
	e.push(item{flush: done})
	e.mu.Unlock()
	return wait(done)
}

func wait(done chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-time.After(FlushTimeout):
		return false
	}
}

// End writes the last line: data.type "result" holding the evidence event.
// result is the document the consumer reads (the evidence, or a verify report);
// meta may be nil. It returns the seq of that line, or 0 when the stream
// already failed or the consumer did not take the line in time.
func (e *Emitter) End(result any, meta *output.Meta) int64 {
	e.mu.Lock()
	if e.err != nil || e.ended {
		e.mu.Unlock()
		return 0
	}
	seq := e.seq + 1
	fields := append(e.fields(seq, "", "", nil), output.Field{Key: "event", Value: string(model.EventType("evidence"))}, output.Field{Key: "evidence", Value: result})
	fields = dropKey(fields, "data")
	line, err := output.StreamEnd(e.command, meta, fields...)
	if err != nil {
		e.err = err
		e.mu.Unlock()
		return 0
	}
	e.seq, e.ended = seq, true
	done := make(chan struct{})
	e.push(item{line: line, last: true, flush: done})
	e.mu.Unlock()
	if !wait(done) {
		e.mu.Lock()
		if e.err == nil {
			e.err = errConsumerStalled
		}
		e.mu.Unlock()
		return 0
	}
	if e.Err() != nil {
		return 0
	}
	return seq
}

var errConsumerStalled = errors.New("the stream consumer stopped reading")

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
