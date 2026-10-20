package agentapi

// Purpose: GET /v1/agent/control, the coordinator-to-agent NDJSON stream: cancel, drain and upgrade frames
//          queued by Cancel, Drain and Upgrade (Epic D6, runner-protocol).
// Inputs:  the session header.
// Outputs: 200 and a stream of frames whose seq continues across reconnects of the same session.
// Constraints: while the stream is open the node counts as having a live control stream (Cancel works);
//          a frame that cannot be written goes back to the queue head, so a dropped connection loses no
//          cancel; the stream ends the moment the request context ends (revoke, CloseNode, shutdown).

import "net/http"

func (s *Server) serveControl(w http.ResponseWriter, r *http.Request) {
	q := s.begin(w, r, true)
	if q == nil {
		return
	}
	defer q.end()
	s.mu.Lock()
	q.sess.ctlLive++
	start := q.sess.ctlSeq
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		q.sess.ctlLive--
		s.mu.Unlock()
	}()
	f, err := newFlow(w, q.rc, start)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "E653", "encoder", "cannot open the control stream")
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = q.rc.Flush()
	for {
		m, changed := s.popControl(q.node)
		if m != nil {
			if q.ctx.Err() != nil || f.send(*m) != nil {
				s.pushBackControl(q.node, *m)
				return
			}
			s.mu.Lock()
			q.sess.ctlSeq = f.seq
			s.mu.Unlock()
			continue
		}
		select {
		case <-q.ctx.Done():
			return
		case <-changed:
		}
	}
}
