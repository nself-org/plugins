package agentapi

// Purpose: the job-token seam of the agent carrier (P7-NODE-08): the call a node makes for one lease, with the
//          token it presented, handed to a hook that binds and checks the token before the lease engine
//          sees the frame.
// Inputs:  an authenticated request (certificate serial, session id) and its X-Nself-Job-Token header values
//          `<lease_id>=<token>`, one per lease the agent runs.
// Outputs: Call for Config.Admit; admit answers false when the call is refused and has already told the agent.
// Constraints: the token travels in a request header over mTLS, never in a frame, so it cannot reach a log, an
//          artifact ref or a result; with no hook set every call is admitted (the NODE-20 behaviour); a
//          refused call is dropped and answered with a cancel for that lease that names the refusal, never
//          by ending the stream, so a late frame of a finished lease cannot make the agent reconnect for ever.

import (
	"context"
	"errors"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// HeaderToken carries `<lease_id>=<token>`, once per lease the agent runs, on the stream request.
const HeaderToken = "X-Nself-Job-Token"

// maxTokens bounds the token header values of one request (an agent holds at most its slot count).
const maxTokens = maxHeartbeatLeases

// BlobHandler takes the blob frames of an upload (digest and chunk) and the offset frames of a
// restore (the node's requests and confirmations) and answers with the frames to send back (offset). An error with
// Refusal() true cancels the lease on the agent; any other error fails the stream.
type BlobHandler interface {
	Frame(ctx context.Context, node string, m protocol.Message) (replies []protocol.Message, err error)
}

// Call is one lease-scoped call of an authenticated node.
type Call struct {
	Node    string
	Session string
	Serial  string // hex serial of the client certificate
	LeaseID string
	Epoch   uint64
	Type    string // the frame type: log, artifact-ref, cache-ref, result or heartbeat
	Token   string // what the agent presented for this lease on this request; empty when it presented none
	// Reconcile is true for the heartbeat entry that opens a session: the agent says it kept this lease across the gap.
	Reconcile bool
}

// parseTokens reads the token header values; malformed ones are ignored (the lease then has no token).
func parseTokens(values []string) map[string]string {
	out := map[string]string{}
	for _, v := range values {
		if len(out) >= maxTokens {
			break
		}
		if i := strings.LastIndexByte(v, '='); i > 0 && i < len(v)-1 {
			out[v[:i]] = v[i+1:]
		}
	}
	return out
}

// admit runs the hook for one call. It returns true when the call may go on. A refusal answers the agent with a
// cancel that names the error; an error of the hook itself that is not a refusal is "coordinator_unavailable".
func (s *Server) admit(q *request, f *flow, leaseID string, epoch uint64, typ string, reconcile bool) (ok bool, bad *fatal) {
	if s.cfg.Admit == nil {
		return true, nil
	}
	err := s.cfg.Admit(q.ctx, Call{Node: q.node, Session: q.sess.id, Serial: q.serial, LeaseID: leaseID, Epoch: epoch, Type: typ, Token: q.tokens[leaseID], Reconcile: reconcile})
	if err == nil {
		return true, nil
	}
	return false, s.refuse(q, f, leaseID, epoch, typ, err)
}

// refuse answers a call a hook turned down. A refusal (an error with Refusal() true) cancels the lease on the
// agent with the error text and returns nil; any other error is the coordinator failing.
func (s *Server) refuse(q *request, f *flow, leaseID string, epoch uint64, typ string, err error) *fatal {
	var refusal interface{ Refusal() bool }
	if !errors.As(err, &refusal) || !refusal.Refusal() {
		return s.unavailable(typ, err)
	}
	s.cfg.Logf("agentapi: node %s lease %s refused: %v", q.node, leaseID, err)
	e := epoch
	msg := protocol.Message{Type: "cancel", LeaseID: leaseID, Epoch: &e, Body: protocol.Cancel{LeaseID: leaseID, Reason: err.Error()}}
	if f.send(msg) != nil {
		return &fatal{protocol.CodeInvalid, "write"}
	}
	return nil
}
