package roles

// Purpose: turning a TLS peer certificate or a scoped token into an Actor (Epic D19).
// Inputs:  a verified leaf certificate (the TLS layer, pki.TLSServerConfig, has checked the chain) or the
//          clear token text.
// Outputs: an Actor holding the principal's live grant (certificate) or the token's own role and projects.
// Constraints: a node-kind or coordinator-kind certificate is E687 node_kind (a node is not a user); a
//          denied serial is refused; a user certificate with no live grant has no role; a token is looked
//          up by its sha256, and an expired or revoked token is E687; the token text never appears in an
//          error or a row. Tokens must travel over TLS: the transport enforces that, not this package.

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ActorFromPeer maps a user certificate to its actor. A refusal writes a rate-limited sign-in failure
// row (signin.go).
func (s *Service) ActorFromPeer(ctx context.Context, cert *x509.Certificate) (Actor, error) {
	a, err := s.actorFromPeer(ctx, cert)
	if err != nil {
		who := "certificate:unknown"
		if cert != nil {
			if id, kind := pki.IdentityOf(cert); kind == pki.KindUser && id != "" {
				who = "user:" + id
			}
		}
		s.auditSignInFailure(ctx, who, "certificate", err)
	}
	return a, err
}

func (s *Service) actorFromPeer(ctx context.Context, cert *x509.Certificate) (Actor, error) {
	if cert == nil {
		return Actor{}, fail(ReasonBadCert, "no client certificate")
	}
	id, kind := pki.IdentityOf(cert)
	switch kind {
	case pki.KindUser:
	case pki.KindNode, pki.KindCoordinator:
		return Actor{}, fail(ReasonNodeKind, "a %s certificate is not a user identity", kind)
	default:
		return Actor{}, fail(ReasonBadCert, "the certificate carries no nself-ci identity")
	}
	if now := s.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return Actor{}, fail(ReasonBadCert, "the certificate is outside its validity window")
	}
	serial := hex.EncodeToString(cert.SerialNumber.Bytes())
	denied, err := s.St.PKIIsDenied(ctx, serial)
	if err != nil {
		return Actor{}, err
	}
	if denied {
		return Actor{}, fail(ReasonBadCert, "the certificate is revoked")
	}
	g, ok, err := s.St.GrantGet(ctx, id)
	if err != nil {
		return Actor{}, err
	}
	if !ok {
		return Actor{}, fail(ReasonNoGrant, "user %s holds no role", id)
	}
	return Actor{id: id, kind: KindUser, role: Role(g.Role), projects: g.Projects, serial: serial, notAfter: cert.NotAfter, svc: s}, nil
}

// ActorFromToken maps a scoped token to its actor. A refusal writes a rate-limited sign-in failure row
// (signin.go), so a token guessing run shows in the audit log.
func (s *Service) ActorFromToken(ctx context.Context, token string) (Actor, error) {
	t, ok, err := s.St.TokenByDigest(ctx, digestOf(token))
	if err != nil {
		return Actor{}, err
	}
	if err = s.tokenState(t, ok); err != nil {
		who := "token:unknown"
		if ok {
			who = "token:" + t.ID + ":" + t.Principal
		}
		s.auditSignInFailure(ctx, who, "token", err)
		return Actor{}, err
	}
	return Actor{id: t.Principal, kind: KindToken, role: Role(t.Role), projects: t.Projects, tokenID: t.ID, svc: s}, nil
}

// tokenState is nil for a found, unrevoked, unexpired token.
func (s *Service) tokenState(t store.Token, found bool) error {
	switch {
	case !found:
		return fail(ReasonTokenUnknown, "unknown token")
	case t.RevokedAt != 0:
		return fail(ReasonTokenRevoked, "token %s was revoked", t.ID)
	case s.Now().UnixNano() >= t.ExpiresAt:
		return fail(ReasonTokenExpired, "token %s expired", t.ID)
	}
	return nil
}

func digestOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NonSelfPrincipals counts the live grants, tokens and user certificates issued to anyone but the
// operator itself, for P7-NODE-20's mode check. It needs a Service installed with Use.
func NonSelfPrincipals(ctx context.Context) (int, error) {
	s := current.Load()
	if s == nil {
		return 0, fail(ReasonNoActor, "no role service is installed")
	}
	return s.St.NonSelfPrincipals(ctx)
}
