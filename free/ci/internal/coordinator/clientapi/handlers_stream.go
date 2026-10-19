package clientapi

// Purpose: the read routes that stream or return stored records: GET /v1/runs/{id}/events (NDJSON,
// contract:ci.events), GET /v1/logs/{job} (NDJSON stored log lines) and GET /v1/evidence/{run} (the signed
// document, byte for byte) (Epic D14).
// Inputs:  the run in the path or the `run` query, `from` (first event seq wanted), `after`, `attempt`,
//          `follow`.
// Outputs: NDJSON lines flushed as they exist; the evidence body.
// Constraints: the event stream is rebuilt from run state by detach.Synth, so a run always yields the same
//          records and seqs and a client that reconnects with from=lastSeq+1 gets no duplicate and no gap;
//          a stream never outlives Config.MaxLife; the evidence body is the stored signed bytes, never
//          re-encoded; a log stream carries stored (already redacted) lines only.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// flushWriter flushes after every write, so a line leaves as soon as it exists.
type flushWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if err == nil {
		_ = f.rc.Flush()
	}
	return n, err
}

func startNDJSON(w http.ResponseWriter) flushWriter {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fw := flushWriter{w: w, rc: http.NewResponseController(w)}
	_ = fw.rc.Flush()
	return fw
}

func viewOf(it RunItem) detach.View {
	v := detach.View{RunID: it.ID, State: it.Status}
	for _, j := range it.Jobs {
		v.Jobs = append(v.Jobs, detach.JobState{Name: j.Name, State: j.State, FailureClass: j.FailureClass})
	}
	return v
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	it, err := s.find(r, a, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	release, ok := s.lim.openStream(a.ID())
	if !ok {
		s.fail(w, Failf(ReasonRateLimited, "too many open streams for this principal"))
		return
	}
	defer release()
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	ctx, stop := s.streamContext(r)
	defer stop()
	synth := detach.NewSynth(startNDJSON(w), it.ID, from)
	tick := time.NewTicker(s.cfg.Poll)
	defer tick.Stop()
	for {
		synth.Observe(viewOf(it))
		synth.Flush()
		if detach.Terminal(it.Status) {
			var ev any
			if raw, e := s.cfg.Backend.Evidence(ctx, it.ID); e == nil {
				ev = json.RawMessage(raw)
			}
			synth.Finish(viewOf(it), ev)
			synth.Flush()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		next, ok, e := s.cfg.Backend.Get(ctx, it.ID)
		if e != nil || !ok {
			return // the client reconnects; nothing is invented
		}
		if authorizeRun(ctx, a, next) != nil {
			return // the grant, token or certificate was revoked meanwhile
		}
		it = next
	}
}

// streamContext bounds a stream by Config.MaxLife and by the end of the caller's certificate.
func (s *Server) streamContext(r *http.Request) (context.Context, context.CancelFunc) {
	life := s.cfg.MaxLife
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		if left := time.Until(r.TLS.PeerCertificates[0].NotAfter); left < life {
			life = max(left, time.Second)
		}
	}
	return context.WithTimeout(r.Context(), life)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	q := r.URL.Query()
	it, err := s.find(r, a, q.Get("run"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if it.Project != "" {
		if err = roles.AuthorizeAction(r.Context(), a, roles.ActLogsView, it.Project); err != nil {
			s.fail(w, Failf(ReasonNotFound, "no such run"))
			return
		}
	}
	release, slot := s.lim.openStream(a.ID())
	if !slot {
		s.fail(w, Failf(ReasonRateLimited, "too many open streams for this principal"))
		return
	}
	defer release()
	lq := LogQuery{Run: it.ID, Job: r.PathValue("job"), Follow: q.Get("follow") == "1"}
	lq.Attempt, _ = strconv.Atoi(q.Get("attempt"))
	lq.After, _ = strconv.ParseInt(q.Get("after"), 10, 64)
	if lq.Attempt < 0 || lq.After < 0 || lq.Job == "" {
		s.fail(w, Failf(ReasonBadRequest, "need a job, attempt >= 0 and after >= 0"))
		return
	}
	ctx, stop := s.streamContext(r)
	defer stop()
	var out io.Writer
	enc := func(l logs.Line) error {
		if out == nil {
			out = startNDJSON(w)
		}
		b, e := json.Marshal(l)
		if e != nil {
			return e
		}
		_, e = out.Write(append(b, '\n'))
		return e
	}
	ok, err := s.cfg.Backend.Logs(ctx, lq, enc)
	switch {
	case out != nil: // lines were sent: the status is already 200
	case err != nil:
		s.fail(w, err)
	case !ok:
		s.fail(w, Failf(ReasonNotFound, "no stored logs for this job"))
	default:
		_ = startNDJSON(w)
	}
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	it, err := s.find(r, a, r.PathValue("run"))
	if err != nil {
		s.fail(w, err)
		return
	}
	raw, err := s.cfg.Backend.Evidence(r.Context(), it.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Run-Id", it.ID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}
