package store

// Purpose: the TEAM role rows (Epic D19): live grants per principal, scoped user tokens stored as a
// sha256 digest, and the subject marker of user certificates. Every mutation writes its audit row in the
// same transaction (sealed when the caller passes a chain function).
// Inputs:  Grant, Token values; the audit input of the mutation.
// Outputs: rows, and NonSelfPrincipals' count for P7-NODE-20's mode check.
// Constraints: expand-only migration set trust-roles, no foreign key into another set; a token's clear
//          text never reaches this file (only its digest); subject is "self" (the data dir's own
//          operator) or the named principal; revoked rows stay for the audit trail.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
)

//go:embed migrations/trust-roles/*.sql
var trustRolesMigrations embed.FS

func init() {
	if err := Register("trust-roles", trustRolesMigrations); err != nil {
		panic(err)
	}
}

// SubjectSelf marks an identity issued to the data dir's own operator.
const SubjectSelf = "self"

// Grant is one principal's live role.
type Grant struct {
	ID                                  int64
	Principal, Role, Subject, GrantedBy string
	Projects                            []string
	GrantedAt, RevokedAt                int64
}

// Token is a scoped user token; Digest is the sha256 hex of the clear text.
type Token struct {
	ID, Digest, Principal, Role, Subject, IssuedBy string
	Projects                                       []string
	IssuedAt, ExpiresAt, RevokedAt                 int64
}

// subjectOK: a subject is "self" or the principal's own name, and the name "self" is never a principal
// (it would hide a remote identity from NonSelfPrincipals).
func subjectOK(principal, subject string) bool {
	return subject == SubjectSelf || (subject == principal && principal != SubjectSelf)
}

func encodeProjects(p []string) (string, error) {
	if p == nil {
		p = []string{}
	}
	b, err := json.Marshal(p)
	return string(b), err
}

func decodeProjects(s string) []string {
	var p []string
	_ = json.Unmarshal([]byte(s), &p)
	return p
}

// GrantPut replaces the principal's live grant (the old row is revoked, not edited).
func (s *Store) GrantPut(ctx context.Context, g Grant, in AuditInput) error {
	if g.Principal == "" || g.Role == "" || g.GrantedBy == "" || !subjectOK(g.Principal, g.Subject) {
		return coded("E607", "invalid grant")
	}
	projects, err := encodeProjects(g.Projects)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		if _, err := tx.ExecContext(ctx, "UPDATE trust_grant SET revoked_at=? WHERE principal=? AND revoked_at IS NULL", now, g.Principal); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO trust_grant(principal,role,projects,subject,granted_by,granted_at) VALUES(?,?,?,?,?,?)",
			g.Principal, g.Role, projects, g.Subject, g.GrantedBy, now); err != nil {
			return err
		}
		return s.auditIn(ctx, tx, in)
	})
}

// GrantRevoke ends the principal's live grant and every live token issued to the principal, in one
// transaction with one audit row; false means there was neither (nothing is audited).
func (s *Store) GrantRevoke(ctx context.Context, principal string, in AuditInput) (found bool, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		g, err := tx.ExecContext(ctx, "UPDATE trust_grant SET revoked_at=? WHERE principal=? AND revoked_at IS NULL", now, principal)
		if err != nil {
			return err
		}
		t, err := tx.ExecContext(ctx, "UPDATE trust_token SET revoked_at=? WHERE principal=? AND revoked_at IS NULL", now, principal)
		if err != nil {
			return err
		}
		gn, _ := g.RowsAffected()
		tn, _ := t.RowsAffected()
		if gn+tn == 0 {
			return nil
		}
		found = true
		return s.auditIn(ctx, tx, in)
	})
	return found, err
}

const grantCols = "id,principal,role,projects,subject,granted_by,granted_at,COALESCE(revoked_at,0)"

func scanGrant(r interface{ Scan(...any) error }) (g Grant, err error) {
	var p string
	err = r.Scan(&g.ID, &g.Principal, &g.Role, &p, &g.Subject, &g.GrantedBy, &g.GrantedAt, &g.RevokedAt)
	g.Projects = decodeProjects(p)
	return g, err
}

// GrantGet returns the principal's live grant.
func (s *Store) GrantGet(ctx context.Context, principal string) (Grant, bool, error) {
	g, err := scanGrant(s.readers.QueryRowContext(ctx, "SELECT "+grantCols+" FROM trust_grant WHERE principal=? AND revoked_at IS NULL", principal))
	if errors.Is(err, sql.ErrNoRows) {
		return Grant{}, false, nil
	}
	return g, err == nil, err
}

// GrantList returns the live grants, by principal.
func (s *Store) GrantList(ctx context.Context) ([]Grant, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT "+grantCols+" FROM trust_grant WHERE revoked_at IS NULL ORDER BY principal")
	return pkiCollect(rows, err, func(r *sql.Rows) (Grant, error) { return scanGrant(r) })
}

// TokenPut stores a token's digest row.
func (s *Store) TokenPut(ctx context.Context, t Token, in AuditInput) error {
	if t.ID == "" || t.Digest == "" || t.Principal == "" || t.Role == "" || t.IssuedBy == "" || t.ExpiresAt <= 0 || !subjectOK(t.Principal, t.Subject) {
		return coded("E607", "invalid token")
	}
	projects, err := encodeProjects(t.Projects)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO trust_token(id,digest,principal,role,projects,subject,issued_by,issued_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)",
			t.ID, t.Digest, t.Principal, t.Role, projects, t.Subject, t.IssuedBy, s.now().UnixNano(), t.ExpiresAt); err != nil {
			return err
		}
		return s.auditIn(ctx, tx, in)
	})
}

// TokenRevoke ends a live token by id; false means no such live token.
func (s *Store) TokenRevoke(ctx context.Context, id string, in AuditInput) (found bool, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE trust_token SET revoked_at=? WHERE id=? AND revoked_at IS NULL", s.now().UnixNano(), id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		found = true
		return s.auditIn(ctx, tx, in)
	})
	return found, err
}

const tokenCols = "id,digest,principal,role,projects,subject,issued_by,issued_at,expires_at,COALESCE(revoked_at,0)"

func scanToken(r interface{ Scan(...any) error }) (t Token, err error) {
	var p string
	err = r.Scan(&t.ID, &t.Digest, &t.Principal, &t.Role, &p, &t.Subject, &t.IssuedBy, &t.IssuedAt, &t.ExpiresAt, &t.RevokedAt)
	t.Projects = decodeProjects(p)
	return t, err
}

// TokenByDigest finds a token by the sha256 hex of its clear text (revoked and expired rows included).
func (s *Store) TokenByDigest(ctx context.Context, digest string) (Token, bool, error) {
	t, err := scanToken(s.readers.QueryRowContext(ctx, "SELECT "+tokenCols+" FROM trust_token WHERE digest=?", digest))
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, false, nil
	}
	return t, err == nil, err
}

// TokenByID finds a token by its id.
func (s *Store) TokenByID(ctx context.Context, id string) (Token, bool, error) {
	t, err := scanToken(s.readers.QueryRowContext(ctx, "SELECT "+tokenCols+" FROM trust_token WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, false, nil
	}
	return t, err == nil, err
}

// TokenList returns every token, newest first (revoked and expired ones carry their state).
func (s *Store) TokenList(ctx context.Context) ([]Token, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT "+tokenCols+" FROM trust_token ORDER BY issued_at DESC, id")
	return pkiCollect(rows, err, func(r *sql.Rows) (Token, error) { return scanToken(r) })
}

// CertSubjectPut records whether a user certificate was issued to the operator itself (subject "self")
// or to a named principal.
func (s *Store) CertSubjectPut(ctx context.Context, serial, principal, subject string) error {
	if serial == "" || principal == "" || !subjectOK(principal, subject) {
		return coded("E607", "invalid certificate subject")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO trust_cert_subject(serial,principal,subject) VALUES(?,?,?) ON CONFLICT(serial) DO UPDATE SET subject=excluded.subject, principal=excluded.principal", serial, principal, subject)
		return err
	})
}

// NonSelfPrincipals counts live grants, unexpired unrevoked tokens and live user certificates (whatever
// the role) whose subject is not "self"; a certificate with no subject row counts as non-self.
func (s *Store) NonSelfPrincipals(ctx context.Context) (int, error) {
	now := s.now().UnixNano()
	var g, t, c int
	if err := s.readers.QueryRowContext(ctx, "SELECT COUNT(*) FROM trust_grant WHERE revoked_at IS NULL AND subject<>?", SubjectSelf).Scan(&g); err != nil {
		return 0, err
	}
	if err := s.readers.QueryRowContext(ctx, "SELECT COUNT(*) FROM trust_token WHERE revoked_at IS NULL AND expires_at>? AND subject<>?", now, SubjectSelf).Scan(&t); err != nil {
		return 0, err
	}
	err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM pki_cert c WHERE c.kind='user' AND c.not_after>?
 AND NOT EXISTS (SELECT 1 FROM pki_denied d WHERE d.serial=c.serial)
 AND NOT EXISTS (SELECT 1 FROM trust_cert_subject s WHERE s.serial=c.serial AND s.subject=?)`, now, SubjectSelf).Scan(&c)
	return g + t + c, err
}
