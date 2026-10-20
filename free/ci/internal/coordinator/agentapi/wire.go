package agentapi

// Purpose: the HTTP plumbing shared by the four routes: the authenticated request context, JSON refusals
//          and the outgoing NDJSON flow (one protocol.Encoder, flushed per frame, with write deadlines).
// Inputs:  an http.Request that passed the guard (a node certificate is on the connection).
// Outputs: request (node id, session, cancellable context), refuse, flow.
// Constraints: a refusal body names a code and a reason key, never a header value, token or frame body;
//          every request registers with the node registry so CloseNode and a deny-list hit end it; the
//          session id travels in the X-Nself-Session header returned by hello.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Headers of the agent carrier.
const (
	// HeaderSession carries the session id from the welcome on every request after hello.
	HeaderSession = "X-Nself-Session"
	// HeaderLastSeq carries the last coordinator-received seq the agent believes in on stream resume.
	HeaderLastSeq = "X-Nself-Last-Seq"
	// HeaderWait carries the long-poll wait in seconds (capped by Config.PollTimeout).
	HeaderWait = "X-Nself-Wait"
)

const writeTimeout = 10 * time.Second

// refuse writes a JSON refusal.
func refuse(w http.ResponseWriter, status int, code, reason, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "reason": reason, "message": msg})
}

// request is one authenticated request of a node.
type request struct {
	node string
	sess *session
	// serial is the hex serial of the client certificate; tokens are the job tokens of this request by lease id.
	serial string
	tokens map[string]string
	ctx    context.Context
	rc     *http.ResponseController
	leave  func()
	stop   context.CancelFunc
}

// begin authenticates the request's node from the certificate, binds the session named by the header
// (needSession) and registers the request. It writes the refusal and returns nil when it cannot.
func (s *Server) begin(w http.ResponseWriter, r *http.Request, needSession bool) *request {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		refuse(w, http.StatusUnauthorized, pki.CodeInvalid, pki.ReasonNoCert, "no client certificate")
		return nil
	}
	node, kind := pki.IdentityOf(r.TLS.PeerCertificates[0])
	if kind != pki.KindNode || node == "" {
		refuse(w, http.StatusForbidden, pki.CodeInvalid, pki.ReasonWrongKind, "node certificate required")
		return nil
	}
	var sess *session
	if needSession {
		if sess = s.sessionOf(node, r.Header.Get(HeaderSession)); sess == nil {
			refuse(w, http.StatusConflict, protocol.CodeInvalid, "no_session", "send hello first")
			return nil
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	conn, _ := r.Context().Value(connKey{}).(net.Conn)
	q := &request{node: node, sess: sess, ctx: ctx, rc: http.NewResponseController(w), stop: cancel,
		serial: hex.EncodeToString(r.TLS.PeerCertificates[0].SerialNumber.Bytes()), tokens: parseTokens(r.Header.Values(HeaderToken))}
	if sess == nil { // hello: no session yet, only the connection is tracked
		q.leave = s.trackConn(node, conn)
		return q
	}
	q.leave = s.enter(node, sess, conn, cancel)
	return q
}

// trackConn registers a connection for CloseNode and returns its release.
func (s *Server) trackConn(node string, conn net.Conn) func() {
	if conn == nil {
		return func() {}
	}
	s.mu.Lock()
	s.addConn(node, s.state(node), conn)
	s.mu.Unlock()
	return func() {}
}

func (q *request) end() {
	q.stop()
	q.leave()
}

// flow is an outgoing NDJSON stream: seq continues from start, each frame is flushed.
type flow struct {
	w   http.ResponseWriter
	rc  *http.ResponseController
	enc *protocol.Encoder
	seq uint64
}

func newFlow(w http.ResponseWriter, rc *http.ResponseController, start uint64) (*flow, error) {
	enc, err := protocol.NewResumedEncoder(w, start, start)
	if err != nil {
		return nil, err
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	return &flow{w: w, rc: rc, enc: enc, seq: start}, nil
}

// send writes one frame (V and Seq are filled in) and flushes it.
func (f *flow) send(m protocol.Message) error {
	m.V, m.Seq = 1, f.seq+1
	_ = f.rc.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := f.enc.Encode(m); err != nil {
		return err
	}
	f.seq = m.Seq
	return f.rc.Flush()
}

// sendReject writes the terminal reject frame of a flow.
func (f *flow) sendReject(code, reason string) {
	_ = f.send(protocol.Message{Type: "reject", Body: protocol.Reject{Code: code, Reason: reason}})
}
