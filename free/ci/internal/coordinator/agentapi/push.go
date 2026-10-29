package agentapi

// Purpose: the coordinator-to-agent push of a lease's cache restore (P7-CACHE-21, Epic CACHE D7, carrier B):
//          digest and chunk frames ride the node's control stream, the node answers with upstream `offset`
//          frames, which reach the BlobHandler when it also implements OffsetHandler.
// Inputs:  Push(ctx, node, message); an upstream offset frame of a lease the node holds.
// Outputs: a message queued on the node's control stream; the offset handed to the handler.
// Constraints: Push waits while pushWindow messages are undelivered (backpressure, so a large blob never
//          fills the queue), ends with ctx, and fails when the node has no live control stream; an offset
//          passes the same ownership, token and epoch fence as every lease-scoped frame.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// pushWindow is how many pushed messages may wait for delivery on one node.
const pushWindow = 16

// OffsetHandler is implemented by a BlobHandler that takes the node's offset answers to a restore push.
type OffsetHandler interface {
	Offset(ctx context.Context, node string, m protocol.Message) error
}

// Push queues m for the node's control stream, waiting while the window is full.
func (s *Server) Push(ctx context.Context, nodeID string, m protocol.Message) error {
	for {
		queued, err := s.tryPush(nodeID, m)
		if queued || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// tryPush checks the window and queues in one step under the registry lock, so concurrent
// pushers never overshoot pushWindow.
func (s *Server) tryPush(nodeID string, m protocol.Message) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.nodes[nodeID]
	if ns == nil || ns.sess == nil || ns.sess.ctlLive == 0 {
		return false, fmt.Errorf("%w: %s", ErrNoControlStream, nodeID)
	}
	if len(ns.ctl) >= pushWindow {
		return false, nil
	}
	ns.ctl = append(ns.ctl, m)
	ns.signal()
	return true, nil
}
