package store

// Purpose: the operator policy rows and pins (contract:ci.policy v1, Epic D15).
// Rows hold the user, team and project scopes; a pin names the protected-branch
// commit an admin chose. P7-TRUST-26 writes through ApplyWrite so a change and
// its audit row commit in one transaction.
// Inputs: scope, subject and key names; JSON values.
// Outputs: PolicyRow and PolicyPin values.
// Constraints: expand-only migration set trust-policy, no foreign key into
// another set; this file checks only the shape of a write (the key registry
// and the tighten-only rule belong to the policy package); SQL stays here.

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

//go:embed migrations/trust-policy/*.sql
var trustPolicyMigrations embed.FS

func init() {
	if err := Register("trust-policy", trustPolicyMigrations); err != nil {
		panic(err)
	}
}

// PolicyRow is one operator policy value.
type PolicyRow struct {
	Scope, Subject, Key string
	Value               json.RawMessage
	UpdatedBy           string
	UpdatedAt           int64
}

// PolicyPin is an admin pin of the protected-branch commit.
type PolicyPin struct {
	Subject, SHA, PinnedBy string
	PinnedAt               int64
}

// Policy write operations.
const (
	PolicySet   = "set"
	PolicyUnset = "unset"
	PolicyPinOp = "pin"
	PolicyClear = "unpin"
)

// PolicyWrite is one change to the policy rows or pins.
type PolicyWrite struct {
	Op, Scope, Subject, Key string
	Value                   json.RawMessage
	SHA, Actor              string
}

// Tx is the write transaction an ApplyWrite callback runs in. It exposes only
// what a policy write may add to the same commit.
type Tx struct {
	s   *Store
	ctx context.Context
	tx  *sql.Tx
}

// AppendAudit adds the audit row inside the policy write's transaction.
func (t *Tx) AppendAudit(actor, action, target, detail string) error {
	return t.s.appendAudit(t.ctx, t.tx, actor, action, target, detail)
}

// maxPolicyValue bounds one stored value.
const maxPolicyValue = 64 << 10

var policyScopes = map[string]bool{"user": true, "team": true, "project": true}
var policySHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func checkPolicyWrite(w PolicyWrite) error {
	if w.Actor == "" || w.Subject == "" {
		return coded("E607", "policy write needs an actor and a subject")
	}
	switch w.Op {
	case PolicySet, PolicyUnset:
		if !policyScopes[w.Scope] || w.Key == "" || strings.ContainsAny(w.Key, " \t\r\n") {
			return coded("E607", "invalid policy row")
		}
		if w.Op == PolicySet && (!json.Valid(w.Value) || len(w.Value) > maxPolicyValue || bytes.Equal(bytes.TrimSpace(w.Value), []byte("null"))) {
			return coded("E607", "policy value must be JSON, not null, and at most 64 KiB")
		}
	case PolicyPinOp:
		if !policySHA.MatchString(w.SHA) {
			return coded("E607", "invalid pin")
		}
	case PolicyClear:
	default:
		return coded("E607", "unknown policy write")
	}
	return nil
}

// PolicyRows returns the rows of one scope and subject as this transaction sees
// them, ordered by key. The write connection is single and the transaction takes
// the write lock when it opens, so a check made through it cannot interleave
// with another writer.
func (t *Tx) PolicyRows(ctx context.Context, scope, subject string) ([]PolicyRow, error) {
	rows, err := t.tx.QueryContext(ctx, policyRowsQuery, scope, subject)
	if err != nil {
		return nil, err
	}
	return scanPolicyRows(rows, scope, subject)
}

// ApplyWrite applies w and then calls inTx in the same transaction. An error
// from either rolls back both.
func (s *Store) ApplyWrite(ctx context.Context, w PolicyWrite, inTx func(tx *Tx) error) error {
	return s.ApplyCheckedWrite(ctx, w, nil, inTx)
}

// ApplyCheckedWrite is ApplyWrite with check run first, inside the write
// transaction: a check that reads the stored rows through tx decides on the
// state the write lands on. An error from check, the write or inTx rolls back
// everything; check must not write through another handle of the store (the
// single writer connection is held), so a refusal that needs an audit row is
// returned and recorded after the call.
func (s *Store) ApplyCheckedWrite(ctx context.Context, w PolicyWrite, check, inTx func(tx *Tx) error) error {
	if err := checkPolicyWrite(w); err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		if check != nil {
			if err := check(&Tx{s: s, ctx: ctx, tx: tx}); err != nil {
				return err
			}
		}
		var err error
		switch w.Op {
		case PolicySet:
			_, err = tx.ExecContext(ctx, `INSERT INTO trust_policy_row(scope,subject,key,value_json,updated_by,updated_at) VALUES(?,?,?,?,?,?)
 ON CONFLICT(scope,subject,key) DO UPDATE SET value_json=excluded.value_json, updated_by=excluded.updated_by, updated_at=excluded.updated_at`,
				w.Scope, w.Subject, w.Key, string(w.Value), w.Actor, now)
		case PolicyUnset:
			_, err = tx.ExecContext(ctx, `DELETE FROM trust_policy_row WHERE scope=? AND subject=? AND key=?`, w.Scope, w.Subject, w.Key)
		case PolicyPinOp:
			_, err = tx.ExecContext(ctx, `INSERT INTO trust_policy_pin(subject,sha,pinned_by,pinned_at) VALUES(?,?,?,?)
 ON CONFLICT(subject) DO UPDATE SET sha=excluded.sha, pinned_by=excluded.pinned_by, pinned_at=excluded.pinned_at`,
				w.Subject, w.SHA, w.Actor, now)
		case PolicyClear:
			_, err = tx.ExecContext(ctx, `DELETE FROM trust_policy_pin WHERE subject=?`, w.Subject)
		}
		if err != nil {
			return err
		}
		if inTx == nil {
			return nil
		}
		return inTx(&Tx{s: s, ctx: ctx, tx: tx})
	})
}

// PolicyRows returns the rows of one scope and subject, ordered by key.
func (s *Store) PolicyRows(ctx context.Context, scope, subject string) ([]PolicyRow, error) {
	rows, err := s.readers.QueryContext(ctx, policyRowsQuery, scope, subject)
	if err != nil {
		return nil, err
	}
	return scanPolicyRows(rows, scope, subject)
}

const policyRowsQuery = `SELECT key,value_json,updated_by,updated_at FROM trust_policy_row WHERE scope=? AND subject=? ORDER BY key`

func scanPolicyRows(rows *sql.Rows, scope, subject string) ([]PolicyRow, error) {
	defer func() { _ = rows.Close() }()
	var out []PolicyRow
	for rows.Next() {
		r := PolicyRow{Scope: scope, Subject: subject}
		var v string
		if err := rows.Scan(&r.Key, &v, &r.UpdatedBy, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Value = json.RawMessage(v)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PolicyPinFor returns the pin of a subject; ok is false when there is none.
func (s *Store) PolicyPinFor(ctx context.Context, subject string) (PolicyPin, bool, error) {
	p := PolicyPin{Subject: subject}
	err := s.readers.QueryRowContext(ctx, `SELECT sha,pinned_by,pinned_at FROM trust_policy_pin WHERE subject=?`, subject).Scan(&p.SHA, &p.PinnedBy, &p.PinnedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyPin{}, false, nil
	}
	return p, err == nil, err
}
