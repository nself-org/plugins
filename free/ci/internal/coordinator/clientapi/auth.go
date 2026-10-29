package clientapi

// Purpose: who is calling (Epic D14, D19): a user certificate or a scoped token over TLS, or the operator
// on the local socket.
// Inputs:  the request (TLS state, Authorization header).
// Outputs: a roles.Actor or an E664 error with a reason key.
// Constraints: a certificate wins over a token; a node or coordinator certificate is E664 node_kind even
//          though the handshake let it through (the user listener checks kinds here, so the refusal has a
//          body); a token on a connection without TLS is E664 plain_http; a token scoped to no project is
//          E664 unscoped_token; the token text is read once and never logged or echoed.

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func asE664(err error, out **Error) bool { return errors.As(err, out) }

// authenticate resolves the caller of req.
func (s *Server) authenticate(req *http.Request) (roles.Actor, error) {
	if s.cfg.Roles == nil {
		return roles.Actor{}, Failf(ReasonInternal, "no role service is configured")
	}
	if s.cfg.Local {
		return s.cfg.Roles.Operator(s.cfg.Operator), nil
	}
	if req.TLS == nil {
		return roles.Actor{}, Failf(ReasonPlainHTTP, "the user API serves TLS only")
	}
	ctx := roles.WithPeer(req.Context(), peerHost(req))
	if len(req.TLS.PeerCertificates) > 0 {
		a, err := s.cfg.Roles.ActorFromPeer(ctx, req.TLS.PeerCertificates[0])
		return a, mapRoles(err)
	}
	tok, ok := bearer(req)
	if !ok {
		return roles.Actor{}, Failf(ReasonNoCredential, "present a user certificate or a bearer token")
	}
	a, err := s.cfg.Roles.ActorFromToken(ctx, tok)
	if err != nil {
		return roles.Actor{}, mapRoles(err)
	}
	if len(a.Projects()) == 0 {
		return roles.Actor{}, Failf(ReasonUnscopedToken, "the token names no project")
	}
	return a, nil
}

// peerHost is the address the request came from, without the port.
func peerHost(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

// bearer reads the Authorization header; only the Bearer scheme counts.
func bearer(req *http.Request) (string, bool) {
	h := strings.TrimSpace(req.Header.Get("Authorization"))
	if len(h) <= len("Bearer ") || !strings.EqualFold(h[:len("Bearer ")], "Bearer ") {
		return "", false
	}
	t := strings.TrimSpace(h[len("Bearer "):])
	return t, t != ""
}

// mapRoles turns an E687 error of the role service into the E664 of the user API, with the same reason.
func mapRoles(err error) error {
	if err == nil {
		return nil
	}
	if r := roles.ReasonOf(err); r != "" {
		if r == roles.ReasonBadCert {
			r = ReasonBadCertificate
		}
		return Failf(r, "the credential was refused")
	}
	return err
}
