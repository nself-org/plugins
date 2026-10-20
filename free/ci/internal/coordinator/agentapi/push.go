package agentapi

// Purpose: Push, the coordinator-to-agent transfer frames (digest and chunk of a cache restore, Epic
//          CACHE D7, P7-CACHE-21) on the node's control stream.
// Inputs:  a frame whose envelope names the lease and its epoch.
// Outputs: the frame on the control stream, in order with the cancels queued before it.
// Constraints: only the transfer frames are accepted; a call waits while the node has pushHighWater
//          undelivered frames, so a restore never queues more than a few chunks in memory, and ends
//          with the caller's context; a node with no live control stream is an error (the caller
//          degrades, the node falls back to a cold miss).

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// pushHighWater bounds the control frames queued for one node while a blob is being pushed.
const pushHighWater = 4

// pushPoll is how often a full queue is looked at again.
const pushPoll = 5 * time.Millisecond

// Push queues one digest or chunk frame for the node.
func (s *Server) Push(ctx context.Context, nodeID string, m protocol.Message) error {
	if m.Type != "digest" && m.Type != "chunk" || m.LeaseID == "" || m.Epoch == nil {
		return fmt.Errorf("agentapi: push takes a lease-scoped digest or chunk frame")
	}
	for {
		s.mu.Lock()
		ns := s.nodes[nodeID]
		switch {
		case ns == nil || ns.sess == nil || ns.sess.ctlLive == 0:
			s.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrNoControlStream, nodeID)
		case len(ns.ctl) < pushHighWater:
			ns.ctl = append(ns.ctl, m)
			ns.signal()
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		t := time.NewTimer(pushPoll)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// RefusalReporter is implemented by a BlobHandler that wants to hear of an offset frame (a restore
// request) the server turned away before it reached Frame: a stale epoch or a denied token.
type RefusalReporter interface {
	Refused(ctx context.Context, node string, m protocol.Message, reason string)
}

// Reasons passed to RefusalReporter.Refused.
const (
	RefusedStale  = "stale_epoch"
	RefusedDenied = "denied"
)

// reportRefused tells the blob handler about a refused restore request.
func (s *Server) reportRefused(q *request, m protocol.Message, reason string) {
	if _, ok := m.Body.(*protocol.Offset); !ok || s.cfg.Blobs == nil {
		return
	}
	if r, ok := s.cfg.Blobs.(RefusalReporter); ok {
		r.Refused(q.ctx, q.node, m, reason)
	}
}
