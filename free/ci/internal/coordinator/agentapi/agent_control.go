package agentapi

// Purpose: the rest of what `nself ci nodes upgrade` needs from a connected pull agent (update.AgentControl, D-1383): the agent
//          version of its last hello, the jobs it reports running, and a wait for the hello after an upgrade. With Upgrade and
//          WaitUpgradeResult (upgrade.go) the Server is the coordinator process's AgentControl.
// Inputs:  every accepted hello (version, slots used) and heartbeat (slots used).
// Outputs: AgentVersion, ActiveJobs, WaitHello.
// Constraints: held in memory per node and dropped with the node (CloseNode); the hello time is the wall clock (time.Now), the
//          clock the caller's since comes from; ActiveJobs answers only for a node with a live session, so an agent that is not
//          connected is never taken for an idle one.

import (
	"context"
	"fmt"
	"time"
)

// helloNote is what the last hello and heartbeat of a node said.
type helloNote struct {
	version string
	at      time.Time
	used    int
}

// noteHello records an accepted hello and wakes WaitHello.
func (s *Server) noteHello(nodeID, version string, used int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(nodeID)
	ns.hello = helloNote{version: version, at: time.Now(), used: used}
	ns.signal()
}

// noteSlots records the slot use a heartbeat reported.
func (s *Server) noteSlots(nodeID string, used int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state(nodeID).hello.used = used
}

// AgentVersion is the version the node reported in its last hello.
func (s *Server) AgentVersion(_ context.Context, nodeID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ns := s.nodes[nodeID]; ns != nil && ns.hello.version != "" {
		return ns.hello.version, nil
	}
	return "", fmt.Errorf("agentapi: node %s has said no hello to this coordinator", nodeID)
}

// ActiveJobs is the number of slots the node reported in use (its last heartbeat, or its hello before the first one).
func (s *Server) ActiveJobs(_ context.Context, nodeID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.nodes[nodeID]
	if ns == nil || ns.sess == nil {
		return 0, fmt.Errorf("%w: %s", ErrNoControlStream, nodeID)
	}
	return ns.hello.used, nil
}

// WaitHello waits up to wait for a hello of the node after since and returns the agent version in it.
func (s *Server) WaitHello(ctx context.Context, nodeID string, since time.Time, wait time.Duration) (string, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		ns := s.state(nodeID)
		if h := ns.hello; h.at.After(since) {
			s.mu.Unlock()
			return h.version, nil
		}
		changed := ns.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return "", fmt.Errorf("agentapi: node %s said no hello within %s", nodeID, wait)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
