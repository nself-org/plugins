package agentapi

// Purpose: POST /v1/agent/hello: version negotiation (E652), node-key pinning (E660 node_key_changed) and
//          the welcome that opens a session (Epic D5, D6, runner-protocol).
// Inputs:  one hello frame (seq 1) from a node certificate; the pinned node key and age recipient.
// Outputs: one welcome or reject frame (seq 1); on welcome the X-Nself-Session header and a new session
//          that replaces any earlier one of the node.
// Constraints: the body is capped at one frame (1 MiB) and decoded strictly; the node key is pinned on
//          the first hello of a node and compared in constant time later (pki.CA.CheckNodeKey); a
//          rejected hello creates no session and pins nothing; welcome carries heartbeat_ms and
//          agent_ttl_ms = coordinator TTL - 2 x heartbeat (Epic D6).

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (s *Server) serveHello(w http.ResponseWriter, r *http.Request) {
	q := s.begin(w, r, false)
	if q == nil {
		return
	}
	defer q.end()
	body := http.MaxBytesReader(w, r.Body, protocol.MaxFrame+1)
	msg, err := protocol.NewDecoder(body, protocol.CarrierPolicy{Carrier: "B"}).Decode()
	var hello *protocol.Hello
	if err == nil {
		var ok bool
		if hello, ok = msg.Body.(*protocol.Hello); !ok || msg.Type != "hello" {
			err = &protocol.Error{Code: protocol.CodeInvalid, Reason: "frame_type"}
		}
	}
	if err != nil {
		code, reason := protocol.CodeInvalid, "decode"
		var pe *protocol.Error
		if errors.As(err, &pe) {
			code, reason = pe.Code, pe.Reason
		} else if errors.Is(err, io.EOF) {
			reason = "empty"
		}
		refuse(w, http.StatusBadRequest, code, reason, "hello frame refused")
		return
	}
	pinned := hello.AgeRecipient
	if s.cfg.PinnedAge != nil {
		if pinned, err = s.cfg.PinnedAge(q.ctx, q.node); err != nil {
			refuse(w, http.StatusServiceUnavailable, protocol.CodeInvalid, "store", "pinned recipient unavailable")
			return
		}
	}
	reply, _ := protocol.AcceptHello(*hello, pinned, s.cfg.Versions, s.cfg.MinAgentVersion, "", int(s.cfg.Heartbeat.Milliseconds()), int(s.cfg.agentTTL().Milliseconds()))
	if rej, ok := reply.(protocol.Reject); ok {
		s.writeHelloReject(w, rej)
		return
	}
	if rej := s.checkNodeKey(r, q.node, hello.NodeKey); rej != nil {
		s.writeHelloReject(w, *rej)
		return
	}
	sess := s.startSession(q.node)
	welcome := reply.(protocol.Welcome)
	welcome.SessionID = sess.id
	w.Header().Set(HeaderSession, sess.id)
	f, ferr := newFlow(w, q.rc, 0)
	if ferr != nil {
		refuse(w, http.StatusInternalServerError, protocol.CodeInvalid, "encoder", "cannot answer")
		return
	}
	if f.send(protocol.Message{Type: "welcome", Body: welcome}) != nil {
		s.cfg.Logf("agentapi: welcome to %s not delivered", q.node)
	}
}

// checkNodeKey validates the offered node key and pins or compares it. Nil means accepted.
func (s *Server) checkNodeKey(r *http.Request, node string, k protocol.NodeKey) *protocol.Reject {
	bad := func(reason string) *protocol.Reject {
		return &protocol.Reject{Code: pki.CodeInvalid, Reason: reason}
	}
	raw, err := base64.StdEncoding.DecodeString(k.Public)
	if k.Alg != "ed25519" || err != nil || len(raw) != ed25519.PublicKeySize || k.KeyID != pki.NodeKeyID(raw) {
		return bad(pki.ReasonBadCSR)
	}
	if err = s.cfg.CA.CheckNodeKey(r.Context(), node, ed25519.PublicKey(raw)); err != nil {
		if reason := pki.ReasonOf(err); reason != "" {
			return bad(reason)
		}
		s.cfg.Logf("agentapi: node key check for %s failed: %v", node, err)
		return bad("store")
	}
	return nil
}

func (s *Server) writeHelloReject(w http.ResponseWriter, rej protocol.Reject) {
	status := http.StatusForbidden
	if rej.Code == protocol.CodeVersion {
		status = http.StatusUpgradeRequired
	}
	rej.MinAgentVersion = firstNonEmpty(rej.MinAgentVersion, s.cfg.MinAgentVersion)
	f, err := newFlow(w, http.NewResponseController(w), 0) // sets the headers; the body is a reject frame
	if err != nil {
		refuse(w, status, rej.Code, rej.Reason, "hello rejected")
		return
	}
	w.WriteHeader(status)
	_ = f.send(protocol.Message{Type: "reject", Body: rej})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
