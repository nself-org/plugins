package agentapi

// Purpose: what the coordinator does with each frame an agent sends on the stream (Epic D6): heartbeats
//          renew leases, the first heartbeat of a session reconciles the leases the agent kept across the
//          gap, lease-scoped frames are fenced by epoch before they reach the sink.
// Inputs:  a decoded protocol.Message; the lease engine and the sink of the Config.
// Outputs: acks and stale-epoch cancels on the stream's response flow; nil or a *fatal that ends the stream.
// Constraints: running -> renewed (first heartbeat: Reconnect) and self_fenced -> lost with reason
//          agent.self_fenced are the lease engine's; a result is fenced, handed to the sink, then
//          committed with Leases.Result, in that order, so a stale or failed result never half-applies;
//          a stale epoch (E662) is never an error to the stream, it is answered with a cancel; frame
//          types that this carrier does not use are a protocol error (E653 frame_type).

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// maxHeartbeatLeases bounds the lease entries of one heartbeat (an agent holds at most its slot count).
const maxHeartbeatLeases = 256

const staleReason = "E662 stale epoch: stop the job and clean its workspace"

// sinkCommits says whether the sink commits a result through the lease engine itself (P7-NODE-08: the run's sink).
func sinkCommits(sink interface{}) bool {
	c, ok := sink.(interface{ CommitsResult() bool })
	return ok && c.CommitsResult()
}

func isStale(err error) bool {
	var se *store.Error
	return errors.As(err, &se) && se.Code == "E662"
}

func epochArg(e uint64) (int64, bool) { return int64(e), e != 0 && e <= math.MaxInt64 }

// holder says how a lease relates to the node that sent a frame.
type holder int

const (
	heldByOther  holder = iota // exists and belongs to another node (or ownership cannot be proven): refuse
	heldByNode                 // the node holds it
	unknownLease               // the coordinator has no such lease (forgotten after a restore): E662, stop the job
)

// owns says whether the lease belongs to the node that sent the frame: a node never speaks for another
// node's lease (a lease id is derived from its attempt and an epoch is guessable). Fail closed: without a
// store, or on any read error, nobody owns it. A proven lease is remembered for the session.
func (s *Server) owns(q *request, leaseID string) holder {
	s.mu.Lock()
	ok := q.sess.owned[leaseID]
	s.mu.Unlock()
	if ok {
		return heldByNode
	}
	if s.cfg.Store == nil {
		return heldByOther
	}
	ctx, cancel := context.WithTimeout(q.ctx, 2*time.Second)
	defer cancel()
	l, err := s.cfg.Store.LeaseGet(ctx, leaseID)
	switch {
	case isStale(err):
		return unknownLease
	case err != nil || l.RunnerID != q.node:
		return heldByOther
	}
	s.mu.Lock()
	q.sess.owned[leaseID] = true
	s.mu.Unlock()
	return heldByNode
}

func (s *Server) unavailable(op string, err error) *fatal {
	s.cfg.Logf("agentapi: %s: %v", op, err)
	return &fatal{protocol.CodeInvalid, "coordinator_unavailable"}
}

func (s *Server) handleFrame(q *request, f *flow, m protocol.Message) *fatal {
	ctx := q.ctx
	var act func() error
	switch b := m.Body.(type) {
	case *protocol.Heartbeat:
		return s.heartbeat(q, f, m.Seq, b)
	case *protocol.Ack:
		if b.LeaseID != "" && s.cfg.OnAck != nil && s.owns(q, b.LeaseID) == heldByNode { // only the lease's own node can acknowledge its cancel
			s.cfg.OnAck(q.node, b.LeaseID)
		}
		return nil
	case *protocol.Log:
		act = func() error { return s.cfg.Sink.Log(ctx, *b) }
	case *protocol.ArtifactRef:
		act = func() error { return s.cfg.Sink.ArtifactRef(ctx, *b) }
	case *protocol.CacheRef:
		act = func() error { return s.cfg.Sink.CacheRef(ctx, *b) }
	case *protocol.Result:
		act = func() error { return s.cfg.Sink.Result(ctx, *b) }
	case *protocol.Digest, *protocol.Chunk:
		if s.cfg.Blobs == nil {
			return &fatal{protocol.CodeInvalid, "frame_type"}
		}
		act = func() error { return nil }
	default:
		return &fatal{protocol.CodeInvalid, "frame_type"}
	}
	if m.Epoch == nil { // a lease-scoped frame always names its epoch
		return &fatal{protocol.CodeInvalid, "epoch"}
	}
	epoch, ok := epochArg(*m.Epoch)
	if !ok {
		return &fatal{protocol.CodeInvalid, "epoch"}
	}
	switch s.owns(q, m.LeaseID) {
	case heldByOther:
		s.cfg.Logf("agentapi: node %s sent a %s frame for lease %s it does not hold", q.node, m.Type, m.LeaseID)
		return &fatal{protocol.CodeInvalid, "lease_not_held"}
	case unknownLease:
		return s.sendStale(f, m.LeaseID, *m.Epoch)
	}
	if ok, bad := s.admit(q, f, m.LeaseID, *m.Epoch, m.Type, false); !ok {
		return bad
	}
	err := s.cfg.Leases.Fence(ctx, m.LeaseID, epoch)
	if err == nil {
		err = act()
	}
	switch m.Body.(type) {
	case *protocol.Digest, *protocol.Chunk:
		if err == nil {
			return s.blobFrame(q, f, m)
		}
	}
	_, isResult := m.Body.(*protocol.Result)
	if err == nil && isResult {
		if !sinkCommits(s.cfg.Sink) { // a sink that commits the result itself (the run's) must not be committed twice
			err = s.cfg.Leases.Result(ctx, m.LeaseID, epoch)
		}
		if err == nil {
			s.mu.Lock()
			delete(q.sess.epochs, m.LeaseID)
			s.mu.Unlock()
		}
	}
	switch {
	case isStale(err):
		return s.sendStale(f, m.LeaseID, *m.Epoch)
	case err != nil:
		return s.unavailable("frame "+m.Type, err)
	}
	if isResult || m.Type == "artifact-ref" {
		return s.ack(f, m.Seq, m.LeaseID)
	}
	return nil
}

// blobFrame hands a digest or chunk frame to the blob handler and sends its replies.
func (s *Server) blobFrame(q *request, f *flow, m protocol.Message) *fatal {
	replies, err := s.cfg.Blobs.Frame(q.ctx, q.node, m)
	if err != nil {
		return s.refuse(q, f, m.LeaseID, *m.Epoch, m.Type, err)
	}
	for _, r := range replies {
		if f.send(r) != nil {
			return &fatal{protocol.CodeInvalid, "write"}
		}
	}
	return nil
}

func (s *Server) ack(f *flow, seq uint64, leaseID string) *fatal {
	if f.send(protocol.Message{Type: "ack", Body: protocol.Ack{AckSeq: seq, LeaseID: leaseID}}) != nil {
		return &fatal{protocol.CodeInvalid, "write"}
	}
	return nil
}

// sendStale answers a stale epoch: the agent must stop that lease's job and clean its workspace.
func (s *Server) sendStale(f *flow, leaseID string, epoch uint64) *fatal {
	err := f.send(protocol.Message{Type: "cancel", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Cancel{LeaseID: leaseID, Reason: staleReason}})
	if err != nil {
		return &fatal{protocol.CodeInvalid, "write"}
	}
	return nil
}

// heartbeat renews every running lease the agent reports and acknowledges the heartbeat. The first
// heartbeat of a session reconciles instead: running -> renewed, self_fenced -> lost.
func (s *Server) heartbeat(q *request, f *flow, seq uint64, hb *protocol.Heartbeat) *fatal {
	s.mu.Lock()
	first := !q.sess.reconciled
	q.sess.reconciled = true
	s.mu.Unlock()
	unacked := false
	if len(hb.Leases) > maxHeartbeatLeases {
		return &fatal{protocol.CodeInvalid, "too_many_leases"}
	}
	for _, ls := range hb.Leases {
		epoch, ok := epochArg(ls.Epoch)
		if !ok {
			return &fatal{protocol.CodeInvalid, "epoch"}
		}
		switch s.owns(q, ls.LeaseID) {
		case heldByOther:
			s.cfg.Logf("agentapi: node %s reported lease %s it does not hold", q.node, ls.LeaseID)
			return &fatal{protocol.CodeInvalid, "lease_not_held"}
		case unknownLease:
			if bad := s.sendStale(f, ls.LeaseID, ls.Epoch); bad != nil {
				return bad
			}
			continue
		}
		if ls.State == "running" || ls.State == "self_fenced" {
			if ok, bad := s.admit(q, f, ls.LeaseID, ls.Epoch, "heartbeat", first); !ok {
				if bad != nil {
					return bad
				}
				unacked = true // a lease the gate refused is not renewed, so the agent must fence it on its own clock
				continue
			}
		}
		var err error
		switch {
		case ls.State == "self_fenced":
			err = s.cfg.Leases.Reconnect(q.ctx, ls.LeaseID, epoch, "self_fenced")
		case ls.State == "running" && first:
			err = s.cfg.Leases.Reconnect(q.ctx, ls.LeaseID, epoch, "running")
		case ls.State == "running":
			err = s.cfg.Leases.Renew(q.ctx, ls.LeaseID, epoch, 0)
		default:
			continue
		}
		if isStale(err) {
			if bad := s.sendStale(f, ls.LeaseID, ls.Epoch); bad != nil {
				return bad
			}
			continue
		}
		if err != nil { // never acknowledge a heartbeat whose renewal failed: the agent must fence on its own clock
			s.cfg.Logf("agentapi: heartbeat of %s lease %s: %v", q.node, ls.LeaseID, err)
			unacked = true
			continue
		}
		s.mu.Lock()
		if ls.State == "running" {
			q.sess.epochs[ls.LeaseID] = ls.Epoch
		} else {
			delete(q.sess.epochs, ls.LeaseID)
		}
		s.mu.Unlock()
	}
	if unacked {
		return nil
	}
	return s.ack(f, seq, "")
}
