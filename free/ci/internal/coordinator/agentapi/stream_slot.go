package agentapi

// Purpose: the one-stream-per-session rule of POST /v1/agent/stream (D-1378): a session has at most one stream handler at a
//          time, and a restarted stream takes over from the old one.
// Inputs:  a stream request that passed begin (its session and cancel func).
// Outputs: claimStream: the release to run when the handler returns, or a refusal when the old handler will not return.
// Constraints: the registry mutex guards the slot and no I/O runs under it; the claim waits for the old handler, so the
//          session's last accepted seq is settled before the new stream reads it.

import (
	"context"
	"time"
)

// streamSlot is the live stream of a session: cancel ends it, done closes when its handler has returned.
type streamSlot struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// streamTakeoverWait is how long a restarted stream waits for the handler of the old one to return before it is refused
// (409 stream_busy) and the agent retries; a variable so tests can shorten it.
var streamTakeoverWait = 2 * time.Second

// claimStream makes q the only stream handler of its session (D-1378). A restarted stream replaces the old one: the old
// handler is cancelled and the claim waits, up to streamTakeoverWait, until it has returned, so two handlers never decode
// frames of one session at once and the session's last accepted seq is settled before the new stream reads it. It returns
// the release to run when the handler ends, or false when the old handler did not return in time or q ended first.
func (s *Server) claimStream(q *request) (release func(), ok bool) {
	timer := time.NewTimer(streamTakeoverWait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		cur := q.sess.stream
		if cur == nil {
			mine := &streamSlot{cancel: q.stop, done: make(chan struct{})}
			q.sess.stream = mine
			s.mu.Unlock()
			return func() {
				s.mu.Lock()
				if q.sess.stream == mine {
					q.sess.stream = nil
				}
				s.mu.Unlock()
				close(mine.done)
			}, true
		}
		s.mu.Unlock()
		cur.cancel()
		select {
		case <-cur.done:
		case <-timer.C:
			return nil, false
		case <-q.ctx.Done():
			return nil, false
		}
	}
}
