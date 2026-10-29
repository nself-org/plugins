package roles

// Purpose: managing grants and scoped tokens (Epic D19): `serve admin roles grant|revoke|list` and
// `token issue|revoke|list`.
// Inputs:  an acting Actor (checked for roles.manage / token.manage) and the request.
// Outputs: stored rows; the clear token text exactly once, from IssueToken.
// Constraints: a token is 32 random bytes in base64url, stored only as its sha256; each mutation writes
//          exactly one audit row in its own transaction, built by the pure check (no extra decision row); a
//          refusal writes one "<action>.refused" row with the reason key (signin.go, D-1276);
//          `--self` identities are issued only to Service.Self and record subject "self", all others
//          record the named principal; roles are the three named ones; a project list is explicit or "*".

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Token lifetime bounds.
const (
	MinTokenTTL     = time.Minute
	MaxTokenTTL     = 365 * 24 * time.Hour
	DefaultTokenTTL = 30 * 24 * time.Hour
)

// Request describes a grant or a token.
type Request struct {
	Principal string
	Role      Role
	Projects  []string
	Self      bool
	TTL       time.Duration // tokens only
}

// admin re-reads the acting identity and requires the admin role, so a mutation honours a revocation
// that happened after the actor was resolved.
func (s *Service) admin(ctx context.Context, by Actor) error {
	if by.svc == nil || by.id == "" || by.kind == "" {
		return fail(ReasonNoActor, "no identity")
	}
	by, err := by.refresh(ctx)
	if err != nil {
		return err
	}
	return by.Can(Admin, "")
}

func (s *Service) check(ctx context.Context, by Actor, r Request) (subject string, err error) {
	if err = s.admin(ctx, by); err != nil {
		return "", err
	}
	if !pki.ValidID(r.Principal) {
		return "", fail(ReasonBadRequest, "principal %q is not a valid identifier", r.Principal)
	}
	if r.Principal == store.SubjectSelf && !r.Self {
		return "", fail(ReasonBadRequest, "%q is reserved for the operator's own identity (--self)", r.Principal)
	}
	if r.Role.Rank() == 0 {
		return "", fail(ReasonUnknownRole, "role must be member, release or admin")
	}
	if len(r.Projects) == 0 {
		return "", fail(ReasonBadRequest, "name at least one project, or %s for all", AllProjects)
	}
	for _, p := range r.Projects {
		if p == "" || strings.ContainsAny(p, " \t\r\n,") {
			return "", fail(ReasonBadRequest, "invalid project id %q", p)
		}
	}
	if r.Self {
		if s.Self == "" || r.Principal != s.Self {
			return "", fail(ReasonBadRequest, "--self applies only to the operator's own identity")
		}
		return store.SubjectSelf, nil
	}
	return r.Principal, nil
}

func (s *Service) mutation(by Actor, action, target, detail string) (store.AuditInput, error) {
	return s.Log.Input(audit.Event{Actor: by.Label(), Action: action, Target: target, Detail: detail})
}

// Grant gives (or replaces) a principal's role.
func (s *Service) Grant(ctx context.Context, by Actor, r Request) error {
	subject, err := s.check(ctx, by, r)
	if err != nil {
		return s.refused(ctx, by, "roles.grant", r.Principal, err)
	}
	in, err := s.mutation(by, "roles.grant", r.Principal, fmt.Sprintf("role=%s projects=%s subject=%s", r.Role, strings.Join(r.Projects, ","), subject))
	if err != nil {
		return err
	}
	return s.St.GrantPut(ctx, store.Grant{Principal: r.Principal, Role: string(r.Role), Projects: r.Projects, Subject: subject, GrantedBy: by.Label()}, in)
}

// Revoke ends a principal's grant and every live token issued to the principal; false means there was
// neither.
func (s *Service) Revoke(ctx context.Context, by Actor, principal string) (bool, error) {
	if err := s.admin(ctx, by); err != nil {
		return false, s.refused(ctx, by, "roles.revoke", principal, err)
	}
	in, err := s.mutation(by, "roles.revoke", principal, "grant and live tokens revoked")
	if err != nil {
		return false, err
	}
	return s.St.GrantRevoke(ctx, principal, in)
}

// reader re-reads the acting identity and requires at least the member role.
func (s *Service) reader(ctx context.Context, by Actor) error {
	if by.svc == nil || by.id == "" || by.kind == "" {
		return fail(ReasonNoActor, "no identity")
	}
	by, err := by.refresh(ctx)
	if err != nil {
		return err
	}
	return by.Can(Member, "")
}

// List returns the live grants (member role and above may read them).
func (s *Service) List(ctx context.Context, by Actor) ([]store.Grant, error) {
	if err := s.reader(ctx, by); err != nil {
		return nil, err
	}
	return s.St.GrantList(ctx)
}

// IssueToken creates a scoped token and returns its clear text. The text exists only in the return value.
func (s *Service) IssueToken(ctx context.Context, by Actor, r Request) (string, store.Token, error) {
	subject, err := s.check(ctx, by, r)
	if err != nil {
		return "", store.Token{}, s.refused(ctx, by, "token.issue", r.Principal, err)
	}
	ttl := r.TTL
	if ttl == 0 {
		ttl = DefaultTokenTTL
	}
	if ttl < MinTokenTTL || ttl > MaxTokenTTL {
		return "", store.Token{}, s.refused(ctx, by, "token.issue", r.Principal, fail(ReasonBadRequest, "token lifetime must be between %s and %s", MinTokenTTL, MaxTokenTTL))
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", store.Token{}, err
	}
	clear := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(clear))
	digest := hex.EncodeToString(sum[:])
	t := store.Token{ID: "tok-" + digest[:12], Digest: digest, Principal: r.Principal, Role: string(r.Role), Projects: r.Projects,
		Subject: subject, IssuedBy: by.Label(), ExpiresAt: s.Now().Add(ttl).UnixNano()}
	in, err := s.mutation(by, "token.issue", r.Principal, fmt.Sprintf("id=%s role=%s projects=%s subject=%s expires=%s", t.ID, r.Role, strings.Join(r.Projects, ","), subject, s.Now().Add(ttl).UTC().Format(time.RFC3339)))
	if err != nil {
		return "", store.Token{}, err
	}
	if err = s.St.TokenPut(ctx, t, in); err != nil {
		return "", store.Token{}, err
	}
	return clear, t, nil
}

// RevokeToken ends a live token by id; false means no such live token.
func (s *Service) RevokeToken(ctx context.Context, by Actor, id string) (bool, error) {
	if err := s.admin(ctx, by); err != nil {
		return false, s.refused(ctx, by, "token.revoke", id, err)
	}
	in, err := s.mutation(by, "token.revoke", id, "token revoked")
	if err != nil {
		return false, err
	}
	return s.St.TokenRevoke(ctx, id, in)
}

// Tokens lists every token (the digest is not returned to callers that print it; they use ID).
func (s *Service) Tokens(ctx context.Context, by Actor) ([]store.Token, error) {
	if err := s.reader(ctx, by); err != nil {
		return nil, err
	}
	return s.St.TokenList(ctx)
}
