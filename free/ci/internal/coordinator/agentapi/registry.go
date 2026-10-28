package agentapi

// Purpose: the per-node connection registry of the agent listener: one session per node, the requests
//          and TCP connections it holds open, the leases waiting for the node and the control messages
//          queued for it.
// Inputs:  hello (startSession), request enter/leave, Offer, Cancel, Drain, Upgrade, CloseNode.
// Outputs: the exported API other units call: Cancel, NodesConnected, CloseNode, Offer, Drain, Upgrade.
// Constraints: one mutex guards everything here and no I/O runs under it; CloseNode cancels every request
//          of the node and closes its connections, which is what a revoke relies on (lifecycle session
//          closer) to end long-polls and streams within 1 s; queues are bounded; control messages not
//          yet delivered survive a reconnect (a cancel is idempotent, so a redelivery is harmless).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type connKey struct{}

// Queue bounds: a node that never drains them is refused more work, not given unbounded memory.
const maxQueued = 1024

// ErrNoControlStream is returned by Cancel, Drain and Upgrade when the node has no live control stream.
var ErrNoControlStream = errors.New("agentapi: node has no live control stream")

type session struct {
	id         string
	seen       time.Time
	cancels    map[int]context.CancelFunc
	nextCancel int
	upSeq      uint64 // last accepted seq of the stream (agent to coordinator) direction
	leaseSeq   uint64 // seq of the lease long-poll flow
	ctlSeq     uint64 // seq of the control flow
	epochs     map[string]uint64
	owned      map[string]bool // leases already proven to belong to this node
	reconciled bool            // the first heartbeat of a session reports the leases the agent kept across the gap
	ctlLive    int
	stream     *streamSlot // the one POST /v1/agent/stream handler of the session, nil when none runs
}

// streamSlot is the live stream of a session: cancel ends it, done closes when its handler has returned.
type streamSlot struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// streamTakeoverWait is how long a restarted stream waits for the handler of the old one to return before it is refused
// (409 stream_busy) and the agent retries; a variable so tests can shorten it.
var streamTakeoverWait = 2 * time.Second

type nodeState struct {
	sess    *session
	conns   map[net.Conn]struct{}
	leases  []protocol.Lease
	ctl     []protocol.Message
	changed chan struct{}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// state returns the node's entry; the caller holds s.mu.
func (s *Server) state(node string) *nodeState {
	ns := s.nodes[node]
	if ns == nil {
		ns = &nodeState{conns: map[net.Conn]struct{}{}, changed: make(chan struct{})}
		s.nodes[node] = ns
	}
	return ns
}

// signal wakes everything waiting on the node; the caller holds s.mu.
func (ns *nodeState) signal() {
	close(ns.changed)
	ns.changed = make(chan struct{})
}

// startSession replaces the node's session (a new hello); the requests of the old one are cancelled.
func (s *Server) startSession(node string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	if ns.sess != nil {
		for _, c := range ns.sess.cancels {
			c()
		}
	}
	ns.sess = &session{id: newID(), seen: s.cfg.Now(), cancels: map[int]context.CancelFunc{}, epochs: map[string]uint64{}, owned: map[string]bool{}}
	ns.signal()
	return ns.sess
}

// sessionOf returns the live session when id matches.
func (s *Server) sessionOf(node, id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ns := s.nodes[node]; ns != nil && ns.sess != nil && id != "" && ns.sess.id == id {
		return ns.sess
	}
	return nil
}

// enter registers one request of a session: its cancel func and its connection. The returned leave
// must run when the request ends.
func (s *Server) enter(node string, sess *session, conn net.Conn, cancel context.CancelFunc) (leave func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	s.addConn(node, ns, conn)
	sess.nextCancel++
	n := sess.nextCancel
	sess.cancels[n] = cancel
	sess.seen = s.cfg.Now()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(sess.cancels, n)
		sess.seen = s.cfg.Now()
	}
}

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

// addConn remembers a connection of the node; the caller holds s.mu.
func (s *Server) addConn(node string, ns *nodeState, conn net.Conn) {
	if conn == nil {
		return
	}
	ns.conns[conn] = struct{}{}
	s.connNode[conn] = node
}

// dropConn forgets a closed connection (http.Server.ConnState).
func (s *Server) dropConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if node, ok := s.connNode[conn]; ok {
		delete(s.connNode, conn)
		if ns := s.nodes[node]; ns != nil {
			delete(ns.conns, conn)
		}
	}
}

// CloseNode ends every live request of the node, closes its connections and forgets its session and queues.
func (s *Server) CloseNode(nodeID string) {
	s.mu.Lock()
	ns := s.nodes[nodeID]
	var conns []net.Conn
	if ns != nil {
		if ns.sess != nil {
			for _, c := range ns.sess.cancels {
				c()
			}
		}
		for c := range ns.conns {
			conns = append(conns, c)
		}
		for c := range ns.conns {
			delete(s.connNode, c)
		}
		ns.sess, ns.conns, ns.leases, ns.ctl = nil, map[net.Conn]struct{}{}, nil, nil
		ns.signal()
		delete(s.nodes, nodeID)
	}
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// NodesConnected lists the nodes with a session that has a live request or was seen within the agent TTL.
func (s *Server) NodesConnected() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now, out := s.cfg.Now(), []string{}
	for id, ns := range s.nodes {
		if sess := ns.sess; sess != nil && (len(sess.cancels) > 0 || now.Sub(sess.seen) <= s.cfg.agentTTL()) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Offer queues a lease for the node; its next long-poll on GET /v1/agent/leases receives it.
func (s *Server) Offer(nodeID string, l protocol.Lease) error {
	if nodeID == "" || l.LeaseID == "" || l.Epoch == 0 {
		return errors.New("agentapi: a lease needs a node, a lease id and an epoch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(nodeID)
	if len(ns.leases) >= maxQueued {
		return fmt.Errorf("agentapi: node %s has %d undelivered leases", nodeID, len(ns.leases))
	}
	ns.leases = append(ns.leases, l)
	ns.signal()
	return nil
}

// Cancel queues a cancel for a lease on the node's control stream. It errors when the node has no live
// control stream (the caller then waits for lease expiry, Epic D6).
func (s *Server) Cancel(nodeID, leaseID, reason string) error {
	if leaseID == "" {
		return errors.New("agentapi: cancel needs a lease id")
	}
	epoch, err := s.epochOf(nodeID, leaseID)
	if err != nil {
		return err
	}
	return s.queueControl(nodeID, protocol.Message{Type: "cancel", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Cancel{LeaseID: leaseID, Reason: reason}})
}

// Drain asks the node's agent to stop taking leases and finish by deadline.
func (s *Server) Drain(nodeID string, deadline time.Time) error {
	return s.queueControl(nodeID, protocol.Message{Type: "drain", Body: protocol.Drain{Deadline: deadline.UTC().Format(time.RFC3339)}})
}

// Upgrade asks the node's agent to upgrade to version from manifestURL.
func (s *Server) Upgrade(nodeID, version, manifestURL string) error {
	return s.queueControl(nodeID, protocol.Message{Type: "upgrade", Body: protocol.Upgrade{Version: version, ManifestURL: manifestURL}})
}

func (s *Server) queueControl(nodeID string, m protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.nodes[nodeID]
	if ns == nil || ns.sess == nil || ns.sess.ctlLive == 0 {
		return fmt.Errorf("%w: %s", ErrNoControlStream, nodeID)
	}
	if len(ns.ctl) >= maxQueued {
		return fmt.Errorf("agentapi: node %s has %d undelivered control messages", nodeID, len(ns.ctl))
	}
	ns.ctl = append(ns.ctl, m)
	ns.signal()
	return nil
}

// epochOf is the epoch a cancel for the lease carries: what the node last reported or was given, else the store's.
func (s *Server) epochOf(nodeID, leaseID string) (uint64, error) {
	s.mu.Lock()
	if ns := s.nodes[nodeID]; ns != nil && ns.sess != nil {
		if e, ok := ns.sess.epochs[leaseID]; ok {
			s.mu.Unlock()
			return e, nil
		}
	}
	s.mu.Unlock()
	if s.cfg.Store == nil {
		return 0, fmt.Errorf("agentapi: lease %s is not known on node %s", leaseID, nodeID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	l, err := s.cfg.Store.LeaseGet(ctx, leaseID)
	if err != nil || l.Epoch <= 0 {
		return 0, fmt.Errorf("agentapi: lease %s is not known: %w", leaseID, errOr(err))
	}
	return uint64(l.Epoch), nil
}

func errOr(err error) error {
	if err == nil {
		return errors.New("no such lease")
	}
	return err
}
