package agentapi

// Purpose: GET /v1/agent/leases, the lease long-poll: the node asks for work and gets one lease frame, or
//          204 when nothing arrives within the wait (Epic D6; the lease itself is granted by the scheduler
//          through the lease engine and reaches the node through Offer or the LeaseSource).
// Inputs:  the session header, an optional X-Nself-Wait (seconds, capped by Config.PollTimeout).
// Outputs: 200 with one `lease` frame (envelope lease_id and epoch set), or 204.
// Constraints: the request ends at once when its context ends (revoke, CloseNode, shutdown); a lease
//          that cannot be written goes back to the front of the queue; a lease that was written but never
//          started expires through the lease engine, so nothing runs twice (Epic D6).

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (s *Server) serveLeases(w http.ResponseWriter, r *http.Request) {
	q := s.begin(w, r, true)
	if q == nil {
		return
	}
	defer q.end()
	wait := s.cfg.PollTimeout
	if n, err := strconv.Atoi(r.Header.Get(HeaderWait)); err == nil && n >= 0 && time.Duration(n)*time.Second < wait {
		wait = time.Duration(n) * time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	var poll <-chan time.Time
	if s.cfg.Source != nil {
		t := time.NewTicker(s.cfg.SourcePoll)
		defer t.Stop()
		poll = t.C
	}
	for {
		if s.paused.Load() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		l, changed := s.popLease(q.node, q.sess)
		if l == nil && s.cfg.Source != nil {
			l = s.sourceLease(q)
		}
		if l != nil {
			s.deliverLease(w, q, *l)
			return
		}
		select {
		case <-q.ctx.Done():
			return
		case <-timer.C:
			w.WriteHeader(http.StatusNoContent)
			return
		case <-changed:
		case <-poll:
		}
	}
}

func (s *Server) sourceLease(q *request) *protocol.Lease {
	l, err := s.cfg.Source.Next(q.ctx, q.node)
	if err != nil || l == nil {
		if err != nil && q.ctx.Err() == nil {
			s.cfg.Logf("agentapi: lease source for %s: %v", q.node, err)
		}
		return nil
	}
	s.mu.Lock()
	q.sess.epochs[l.LeaseID] = l.Epoch
	s.mu.Unlock()
	return l
}

// deliverLease writes one lease frame; a failed write puts the lease back.
func (s *Server) deliverLease(w http.ResponseWriter, q *request, l protocol.Lease) {
	if q.ctx.Err() != nil {
		s.pushBackLease(q.node, l)
		return
	}
	s.mu.Lock()
	start := q.sess.leaseSeq
	s.mu.Unlock()
	f, err := newFlow(w, q.rc, start)
	if err == nil {
		epoch := l.Epoch
		err = f.send(protocol.Message{Type: "lease", LeaseID: l.LeaseID, Epoch: &epoch, Body: l})
	}
	if err != nil {
		s.pushBackLease(q.node, l)
		return
	}
	s.mu.Lock()
	q.sess.leaseSeq = f.seq
	s.mu.Unlock()
}
