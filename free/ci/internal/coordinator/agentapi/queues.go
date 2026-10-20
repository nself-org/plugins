package agentapi

// Purpose: the delivery queues of the registry: leases waiting for a node's long-poll and control
//          messages waiting for its control stream.
// Inputs:  pop and push-back calls from the lease and control handlers.
// Outputs: the oldest queued item plus the channel to wait on when the queue is empty.
// Constraints: the registry mutex guards the queues; a popped item that cannot be written is pushed
//          back to the head, so a dropped connection loses nothing.

import "github.com/nself-org/plugins/free/ci/internal/protocol"

// popLease takes the oldest queued lease and the channel to wait on when there is none.
func (s *Server) popLease(node string, sess *session) (*protocol.Lease, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	if len(ns.leases) == 0 {
		return nil, ns.changed
	}
	l := ns.leases[0]
	ns.leases = ns.leases[1:]
	sess.epochs[l.LeaseID] = l.Epoch
	return &l, ns.changed
}

func (s *Server) pushBackLease(node string, l protocol.Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	ns.leases = append([]protocol.Lease{l}, ns.leases...)
}

// popControl takes the oldest queued control message and the channel to wait on when there is none.
func (s *Server) popControl(node string) (*protocol.Message, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	if len(ns.ctl) == 0 {
		return nil, ns.changed
	}
	m := ns.ctl[0]
	ns.ctl = ns.ctl[1:]
	return &m, ns.changed
}

func (s *Server) pushBackControl(node string, m protocol.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(node)
	ns.ctl = append([]protocol.Message{m}, ns.ctl...)
}
