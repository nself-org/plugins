package ssh

// Purpose: the lease phase of a session: send the lease, secrets and checkout,
// then handle every agent frame until the result, a cancel, or session loss.
// Inputs:  the handshaken session.
// Outputs: Sink calls (log, artifact-ref, cache-ref, blobs, result); nil after
// the result, else a *Error.
// Constraints: frames for another lease or epoch are dropped, never forwarded;
// every agent heartbeat is answered with a heartbeat (the ack the agent fence waits
// for, P7-NODE-09) and one that reports this lease also renews it; an agent that sends
// nothing for the coordinator TTL is lost, a frozen one cannot hold ssh open; an EOF or read error before the
// result is a lost attempt (transport.ssh_session_lost) and is never resumed.

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (s *sess) serve(ctx context.Context, bundle string) error {
	if err := s.send("lease", s.lease, true); err != nil {
		return s.lostErr(err)
	}
	if err := s.sendSecrets(ctx); err != nil {
		return err
	}
	uploaded := make(chan error, 1)
	if bundle != "" {
		go func() { uploaded <- s.upload(ctx, bundle) }()
	} else {
		uploaded <- nil
	}
	pending := uploaded
	s.touch()
	// The silence watchdog runs on its own goroutine, so a Sink callback that blocks cannot
	// stop it; it cancels the context the callbacks run under.
	hctx, stopCallbacks := context.WithCancel(ctx)
	defer stopCallbacks()
	silent := make(chan struct{})
	quit := make(chan struct{})
	defer close(quit)
	go s.watchSilence(s.d.coordinatorTTL(), silent, stopCallbacks, quit)
	for {
		select {
		case <-ctx.Done():
			return s.cancelAndDrain(ctx)
		case <-silent:
			_ = s.send("cancel", protocol.Cancel{LeaseID: s.lease.LeaseID, Reason: "agent_silent"}, true)
			return s.lostErr(nil)
		case err := <-pending:
			pending = nil
			if err != nil && ctx.Err() == nil {
				_ = s.send("cancel", protocol.Cancel{LeaseID: s.lease.LeaseID, Reason: "checkout_failed"}, true)
				return err
			}
		case it := <-s.frames:
			if it.err != nil {
				return s.lostErr(it.err)
			}
			done, err := s.handle(hctx, it.m)
			if err != nil && hctx.Err() != nil && ctx.Err() == nil { // the watchdog cancelled a hung callback
				_ = s.send("cancel", protocol.Cancel{LeaseID: s.lease.LeaseID, Reason: "agent_silent"}, true)
				return s.lostErr(nil)
			}
			if err != nil {
				_ = s.send("cancel", protocol.Cancel{LeaseID: s.lease.LeaseID, Reason: "transport_error"}, true)
				return err
			}
			if done {
				return nil
			}
		}
	}
}

// watchSilence closes silent once no frame that proves the agent works on this lease arrived
// for ttl, and cancels the context the Sink callbacks run under.
func (s *sess) watchSilence(ttl time.Duration, silent chan<- struct{}, cancel context.CancelFunc, quit <-chan struct{}) {
	t := time.NewTicker(max(ttl/4, 20*time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-quit:
			return
		case <-t.C:
			if s.quietFor() > ttl {
				cancel()
				close(silent)
				return
			}
		}
	}
}

// touch records that the agent just proved it works on this lease. Times are offsets from
// s.origin on the monotonic clock, so a wall-clock step cannot expire a healthy agent.
func (s *sess) touch() { s.lastAlive.Store(int64(time.Since(s.origin))) }

// quietFor is how long ago touch was last called.
func (s *sess) quietFor() time.Duration {
	return time.Since(s.origin) - time.Duration(s.lastAlive.Load())
}

// noteAlive stamps the arrival of a frame that proves the agent is working on this lease: a
// heartbeat that reports it, or a log, digest, chunk or ref scoped to it. It runs on the reader
// goroutine, before any Sink callback, so a slow Sink does not look like a silent agent. A Sink
// that blocks longer than the coordinator TTL does: the reader stops reading once 64 frames queue,
// the watchdog fires, the callback is cancelled and the attempt is lost (retryable infra); the
// Sink is the fault, and holding the agent's lease open behind it would be worse.
func (s *sess) noteAlive(m protocol.Message) {
	switch b := m.Body.(type) {
	case *protocol.Heartbeat:
		for _, ls := range b.Leases {
			if ls.LeaseID == s.lease.LeaseID && ls.Epoch == s.lease.Epoch {
				s.touch()
			}
		}
	case *protocol.Log:
		if s.ours(m, b.LeaseID, epochOr(epochOf(m), s.lease.Epoch)) {
			s.touch()
		}
	case *protocol.Digest, *protocol.Chunk, *protocol.ArtifactRef, *protocol.CacheRef:
		if s.scoped(m) || ownsRef(s, m) {
			s.touch()
		}
	}
}

func epochOf(m protocol.Message) uint64 {
	if m.Epoch != nil {
		return *m.Epoch
	}
	return 0
}

// ownsRef reports whether an artifact-ref or cache-ref frame names this lease.
func ownsRef(s *sess, m protocol.Message) bool {
	switch b := m.Body.(type) {
	case *protocol.ArtifactRef:
		return s.ours(m, b.LeaseID, epochOr(epochOf(m), s.lease.Epoch))
	case *protocol.CacheRef:
		return s.ours(m, b.LeaseID, epochOr(epochOf(m), s.lease.Epoch))
	}
	return false
}

// lostErr maps the end of the session before a result to the lost reason.
func (s *sess) lostErr(cause error) error {
	if errors.Is(cause, io.EOF) || errors.Is(cause, io.ErrClosedPipe) {
		cause = nil
	}
	e := newErr(CodeTransport, ReasonSessionLost, "ssh session to node %s ended before the result", s.node.Identity.ID)
	e.Err = cause
	return e
}

// cancelAndDrain sends the protocol cancel, waits a short grace for the
// agent's (cancelled) result, and returns the context's error. The deferred
// close then ends the session; the agent kills the job's process group.
func (s *sess) cancelAndDrain(ctx context.Context) error {
	_ = s.send("cancel", protocol.Cancel{LeaseID: s.lease.LeaseID, Reason: "cancelled"}, true)
	bg := context.WithoutCancel(ctx)
	timer := time.NewTimer(s.d.cancelGrace())
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return ctx.Err()
		case it := <-s.frames:
			if it.err != nil {
				return ctx.Err()
			}
			if done, _ := s.handle(bg, it.m); done {
				return ctx.Err()
			}
		}
	}
}

// ours reports whether a lease-scoped frame carries this session's lease and epoch.
func (s *sess) ours(m protocol.Message, leaseID string, epoch uint64) bool {
	if leaseID != s.lease.LeaseID || epoch != s.lease.Epoch {
		return false
	}
	return m.LeaseID == "" || m.LeaseID == s.lease.LeaseID
}

// handle processes one agent frame; done is true after the result was delivered.
func (s *sess) handle(ctx context.Context, m protocol.Message) (done bool, err error) {
	epoch := uint64(0)
	if m.Epoch != nil {
		epoch = *m.Epoch
	}
	switch b := m.Body.(type) {
	case *protocol.Log:
		if s.ours(m, b.LeaseID, epochOr(epoch, s.lease.Epoch)) {
			return false, s.sink.Log(ctx, *b)
		}
	case *protocol.ArtifactRef:
		if s.ours(m, b.LeaseID, epochOr(epoch, s.lease.Epoch)) {
			if !s.committedBlob(b.SHA256) {
				return false, newErr(CodeTransport, ReasonBlob, "artifact-ref names a blob (%s) that was not received in this session", short(b.SHA256))
			}
			return false, s.sink.ArtifactRef(ctx, *b)
		}
	case *protocol.CacheRef:
		if s.ours(m, b.LeaseID, epochOr(epoch, s.lease.Epoch)) {
			for _, cb := range b.Blobs {
				if !s.committedBlob(cb.SHA256) {
					return false, newErr(CodeTransport, ReasonBlob, "cache-ref names a blob (%s) that was not received in this session", short(cb.SHA256))
				}
			}
			return false, s.sink.CacheRef(ctx, *b)
		}
	case *protocol.Result:
		if !s.ours(m, b.LeaseID, b.Epoch) {
			return false, newErr("E662", ReasonFenced, "result for lease %s epoch %d is stale", b.LeaseID, b.Epoch)
		}
		if err := s.sink.Result(ctx, *b); err != nil {
			return false, err
		}
		_ = s.send("ack", protocol.Ack{AckSeq: m.Seq, LeaseID: s.lease.LeaseID}, true)
		return true, nil
	case *protocol.Heartbeat:
		return false, s.onHeartbeat(ctx, m, *b)
	case *protocol.Digest:
		if !s.scoped(m) {
			return false, nil
		}
		return false, s.onDigest(ctx, *b)
	case *protocol.Chunk:
		if !s.scoped(m) {
			return false, nil
		}
		return false, s.onChunk(*b)
	}
	return false, nil
}

// scoped reports whether a frame's envelope names this session's lease and epoch; digest and
// chunk frames of any other attempt are dropped before they can touch a blob.
func (s *sess) scoped(m protocol.Message) bool {
	return m.Epoch != nil && *m.Epoch == s.lease.Epoch && m.LeaseID == s.lease.LeaseID
}

func epochOr(e, def uint64) uint64 {
	if e == 0 {
		return def
	}
	return e
}

func (s *sess) onHeartbeat(ctx context.Context, m protocol.Message, h protocol.Heartbeat) error {
	named := false
	for _, ls := range h.Leases {
		if ls.LeaseID != s.lease.LeaseID || ls.Epoch != s.lease.Epoch {
			continue
		}
		switch ls.State {
		case "refused":
			return newErr("E654", ReasonRefused, "node %s refused lease %s (no free slot or stale epoch)", s.node.Identity.ID, ls.LeaseID)
		case "self_fenced":
			return newErr("E662", ReasonFenced, "agent on node %s fenced lease %s", s.node.Identity.ID, ls.LeaseID)
		}
		named = true
	}
	if named { // only a heartbeat that reports this lease keeps it alive and renewed
		if s.d.Renew != nil {
			if err := s.d.Renew(ctx, s.lease.LeaseID, s.lease.Epoch); err != nil {
				e := newErr("E662", ReasonFenced, "lease %s could not be renewed", s.lease.LeaseID)
				e.Err = err
				return e
			}
		}
	}
	ack := m.Seq
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	reply := protocol.Message{V: 1, Type: "heartbeat", Seq: s.seq + 1, Ack: &ack, Body: protocol.Heartbeat{
		Leases: []protocol.LeaseState{{LeaseID: s.lease.LeaseID, Epoch: s.lease.Epoch, State: "running"}}}}
	if err := s.car.Send(reply); err != nil {
		return s.lostErr(err)
	}
	s.seq++
	return nil
}
