package agentapi

// Purpose: POST /v1/agent/stream, the agent-to-coordinator NDJSON stream: log, artifact-ref, cache-ref,
//          result, heartbeat and ack frames. The response carries the coordinator's frames for this
//          stream: acks of heartbeats, results and artifact refs, and the cancel of a stale epoch.
// Inputs:  the session header; X-Nself-Last-Seq (the last seq the agent believes the coordinator
//          accepted; 0 on a fresh session); frames <= 1 MiB, seq continuing from the session's last
//          accepted frame (protocol.NewResumedDecoder).
// Outputs: 200 and a response flow whose own seq starts at 1 for every POST.
// Constraints: strict decoding; a frame is counted as accepted only after it was handled, so a resume
//          never skips a frame that failed midway; a lease-scoped frame is fenced first (E662 stale
//          epoch becomes a cancel with the workspace-cleanup instruction and the frame is dropped); an
//          idle stream ends after twice the agent TTL; the read side is unblocked when the request
//          context ends, and the watcher goroutine is joined before the handler returns; a session has one stream
//          handler at a time: a new stream cancels the old one and waits for it (claimStream, D-1378), so a slow
//          handler of the old stream never overlaps the new one.

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type fatal struct{ code, reason string }

func (s *Server) serveStream(w http.ResponseWriter, r *http.Request) {
	q := s.begin(w, r, true)
	if q == nil {
		return
	}
	defer q.end()
	release, ok := s.claimStream(q)
	if !ok {
		refuse(w, http.StatusConflict, protocol.CodeInvalid, "stream_busy", "the previous stream of this session is still ending")
		return
	}
	defer release()
	claim := uint64(0)
	if v := r.Header.Get(HeaderLastSeq); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			refuse(w, http.StatusBadRequest, protocol.CodeInvalid, "resume_seq", "last seq is not a number")
			return
		}
		claim = n
	}
	s.mu.Lock()
	last := q.sess.upSeq
	s.mu.Unlock()
	dec, err := protocol.NewResumedDecoder(r.Body, protocol.CarrierPolicy{Carrier: "B"}, last, claim)
	if err != nil || claim != last {
		w.Header().Set(HeaderLastSeq, strconv.FormatUint(last, 10))
		refuse(w, http.StatusConflict, protocol.CodeInvalid, "resume_seq", "the coordinator last accepted another seq")
		return
	}
	_ = q.rc.EnableFullDuplex()
	f, err := newFlow(w, q.rc, 0)
	if err != nil {
		refuse(w, http.StatusInternalServerError, protocol.CodeInvalid, "encoder", "cannot open the stream")
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = q.rc.Flush()

	stopWatch, fin := make(chan struct{}), make(chan struct{})
	go func() { // a blocked body read does not see the context: break it
		defer close(fin)
		select {
		case <-q.ctx.Done():
			_ = q.rc.SetReadDeadline(time.Now())
			_ = r.Body.Close()
		case <-stopWatch:
		}
	}()
	defer func() { close(stopWatch); <-fin }()

	idle := 2 * s.cfg.agentTTL()
	for {
		_ = q.rc.SetReadDeadline(time.Now().Add(idle))
		m, err := dec.Decode()
		if err != nil {
			if q.ctx.Err() == nil && !errors.Is(err, io.EOF) {
				code, reason := protocol.CodeInvalid, "read"
				var pe *protocol.Error
				if errors.As(err, &pe) {
					code, reason = pe.Code, pe.Reason
				}
				f.sendReject(code, reason)
			}
			return
		}
		if bad := s.handleFrame(q, f, m); bad != nil {
			f.sendReject(bad.code, bad.reason)
			return
		}
		s.mu.Lock()
		q.sess.upSeq, q.sess.seen = m.Seq, s.cfg.Now()
		s.mu.Unlock()
	}
}
