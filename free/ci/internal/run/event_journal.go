package run

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"

	"github.com/nself-org/plugins/free/ci/internal/events"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// journalRenderer records the coordinator's progress independently of stdout.
// Log events contain byte counts only; redacted log content stays in the log store.
type journalRenderer struct {
	Renderer
	em *events.Emitter
}
type journalWriter struct {
	ctx context.Context
	st  *store.Store
	run string
}

func (w journalWriter) Write(b []byte) (int, error) {
	var line struct {
		Data struct {
			Seq  int64  `json:"seq"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &line); err != nil {
		return 0, err
	}
	if err := w.st.RecordEvent(w.ctx, w.run, line.Data.Seq, line.Data.Type, b); err != nil {
		return 0, err
	}
	return len(b), nil
}
func journal(ctx context.Context, st *store.Store, run string, r Renderer) Renderer {
	return &journalRenderer{Renderer: r, em: events.New(journalWriter{context.WithoutCancel(ctx), st, run}, "ci", run, run, nil)}
}
func (r *journalRenderer) Event(e Event) {
	switch e.Type {
	case "queued", "planned", "started":
		r.em.Emit(model.EventType(e.Type), e.Job, "", map[string]any{})
	case "log":
		r.em.TryEmit("log", e.Job, "", map[string]any{"bytes": len(e.Text), "ref": nil, "reason": "log.not_kept"})
	case "finished":
		if o := e.Out; o != nil {
			r.em.Emit("check_result", e.Job, o.AttemptID, map[string]any{"state": o.State, "check": o.Check, "required": o.Required, "failure_class": o.FailureClass, "duration_ms": o.Duration.Milliseconds()})
		}
	}
	r.Renderer.Event(e)
}
func (r *journalRenderer) Done(s Summary) {
	r.em.Emit("finished", "", "", map[string]any{"result": s.Result, "jobs": len(s.Jobs)})
	r.em.End(s.Doc, nil)
	if r.em.Err() != nil {
		s.Notes = append(s.Notes, "the run event journal could not be written")
	}
	r.Renderer.Done(s)
}
