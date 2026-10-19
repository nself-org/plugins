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
)

// ActorFromPeer maps a user certificate to its actor.
func (s *Service) ActorFromPeer(ctx context.Context, cert *x509.Certificate) (Actor, error) {
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
	denied, err := s.St.PKIIsDenied(ctx, hex.EncodeToString(cert.SerialNumber.Bytes()))
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
	return Actor{ID: id, Kind: KindUser, Role: Role(g.Role), Projects: g.Projects, svc: s}, nil
}

// ActorFromToken maps a scoped token to its actor.
func (s *Service) ActorFromToken(ctx context.Context, token string) (Actor, error) {
	t, ok, err := s.St.TokenByDigest(ctx, digestOf(token))
	if err != nil {
		return Actor{}, err
	}
	switch {
	case !ok:
		return Actor{}, fail(ReasonTokenUnknown, "unknown token")
	case t.RevokedAt != 0:
		return Actor{}, fail(ReasonTokenRevoked, "token %s was revoked", t.ID)
	case s.Now().UnixNano() >= t.ExpiresAt:
		return Actor{}, fail(ReasonTokenExpired, "token %s expired", t.ID)
	}
	return Actor{ID: t.Principal, Kind: KindToken, Role: Role(t.Role), Projects: t.Projects, TokenID: t.ID, svc: s}, nil
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
