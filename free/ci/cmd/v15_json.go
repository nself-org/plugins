package main

// Purpose: the machine renderers of bare `nself ci` (P7-CI-38, Epic D3):
// `--json` prints one v1 envelope whose data is the evidence document;
// `--json --stream` prints ci.events NDJSON, one envelope per line, the last
// line data.type "result" carrying the evidence. Both register with the run
// package's renderer registry.
// Inputs:  run.Event values and the final run.Summary.
// Outputs: JSON on the writer; nothing else (human text never mixes in).
// Constraints: a renderer never fails the run (a closed pipe stops output,
// not the gate); `log` records carry a byte count and no text; the verify
// command reuses one streamSession so its result line continues the seq.

import (
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/events"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// streamSession is one NDJSON stream; the emitter is made on first use, when
// the run id is known.
type streamSession struct {
	mu      sync.Mutex
	w       io.Writer
	command string
	em      *events.Emitter
}

func (s *streamSession) emitter(runID string) *events.Emitter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.em == nil {
		s.em = events.New(s.w, s.command, runID, runID, nil)
	}
	return s.em
}

// ignoreBrokenPipe makes a write to a closed stdout return EPIPE instead of
// killing the process, so the events emitter stops quietly and deferred
// cleanup (the verify worktree) still runs when a stream consumer goes away.
// It installs a handler rather than SIG_IGN: an ignored signal is inherited
// by every job, a handled one is reset to the default on exec.
var pipeOnce sync.Once

func ignoreBrokenPipe() {
	pipeOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGPIPE)
		go func() {
			for range ch {
			}
		}()
	})
}

// flush waits for the records queued so far to reach the consumer.
func (s *streamSession) flush() {
	s.mu.Lock()
	em := s.em
	s.mu.Unlock()
	if em != nil {
		em.Flush()
	}
}

// flushActiveStream flushes the verify command's stream, if it has one.
func flushActiveStream() {
	activeMu.Lock()
	s := activeSession
	activeMu.Unlock()
	if s != nil {
		s.flush()
	}
}

// activeSession, when set, is the stream the "ci-events" renderer writes to.
var (
	activeMu      sync.Mutex
	activeSession *streamSession
)

func init() {
	cirun.RegisterRenderer("json", func(w io.Writer, o cirun.Options) cirun.Renderer {
		if o.Stream {
			ignoreBrokenPipe()
		}
		return &jsonRenderer{w: w, stream: o.Stream, sess: &streamSession{w: w, command: "ci"}}
	})
	cirun.RegisterRenderer("ci-events", func(w io.Writer, o cirun.Options) cirun.Renderer {
		ignoreBrokenPipe()
		activeMu.Lock()
		defer activeMu.Unlock()
		if activeSession == nil {
			activeSession = &streamSession{w: w, command: "ci"}
		}
		return &jsonRenderer{w: w, stream: true, noEnd: true, sess: activeSession}
	})
}

type jsonRenderer struct {
	mu            sync.Mutex
	w             io.Writer
	stream, noEnd bool
	sess          *streamSession
}

func (r *jsonRenderer) Event(e cirun.Event) {
	if !r.stream || e.RunID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	em := r.sess.emitter(e.RunID)
	switch e.Type {
	case "queued":
		em.Emit("queued", "", "", map[string]any{})
	case "planned":
		em.Emit("planned", "", "", plannedData(e))
	case "started":
		em.Emit("started", e.Job, "", map[string]any{})
	case "log": // best effort: dropped once a backlog builds behind a slow consumer
		em.TryEmit("log", e.Job, "", map[string]any{"bytes": len(e.Text), "ref": nil, "reason": "log.not_kept"})
	case "finished":
		o := e.Out
		if o == nil {
			return
		}
		em.Emit("check_result", e.Job, o.AttemptID, map[string]any{"state": o.State, "check": o.Check, "required": o.Required,
			"reason": string(o.Reason), "failure_class": string(o.FailureClass), "exit_code": o.ExitCode, "duration_ms": o.Duration.Milliseconds()})
		if o.State == cirun.StateCancelled {
			em.Emit("cancelled", e.Job, o.AttemptID, map[string]any{})
		}
	}
}

func plannedData(e cirun.Event) map[string]any {
	jobs := []map[string]any{}
	skipped := []map[string]any{}
	if e.Plan != nil {
		for _, j := range e.Plan.Jobs {
			jobs = append(jobs, map[string]any{"id": j.ID, "check": j.Check, "state": j.State, "reason": string(j.Reason), "blocked": string(j.Blocked)})
		}
		for _, k := range e.Plan.Skipped {
			skipped = append(skipped, map[string]any{"job": k.Job, "reason": string(k.Reason)})
		}
	}
	return map[string]any{"jobs": jobs, "skipped": skipped}
}

func (r *jsonRenderer) Done(s cirun.Summary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var meta *output.Meta
	if len(s.Notes) > 0 {
		meta = &output.Meta{Warnings: s.Notes}
	}
	missing := s.Doc.Schema == ""
	if !r.stream {
		if missing {
			_ = output.WriteError(r.w, "ci", noEvidence(), meta)
			return
		}
		_ = output.WriteData(r.w, "ci", s.Doc, meta)
		return
	}
	em := r.sess.emitter(s.RunID)
	em.Emit("finished", "", "", map[string]any{"result": string(s.Result), "jobs": len(s.Jobs)})
	if r.noEnd {
		return
	}
	if missing {
		if line, err := output.StreamError("ci", noEvidence(), meta); err == nil {
			em.Flush() // the error line comes after the records queued before it
			_, _ = r.w.Write(line)
		}
		return
	}
	em.End(s.Doc, meta)
}

func noEvidence() output.ErrorDetail {
	return output.ErrorDetail{Code: evidence.CodeWorktree, Message: "the run finished but its evidence could not be built", ExitCode: 2, Class: "infra",
		Remediation: "Run `nself ci runs` to see the run, then run again"}
}
