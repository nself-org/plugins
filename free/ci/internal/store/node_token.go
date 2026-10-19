package store

// Purpose: the durable first-use binding of a job token (P7-NODE-08, migration set node-token): the token's jti is
//          bound to the certificate serial and the session that first presented it, so a replay from another
//          certificate or session is refused even after a coordinator restart.
// Inputs:  jti, certificate serial, session id, node id, token exp (unix seconds).
// Outputs: TokenBind (first writer wins, the stored row comes back), TokenCheck, TokenRebind (compare and swap
//          of the session), TokenPrune.
// Constraints: the table holds ids and times only, never a token; one jti has one row; a bind never replaces a
//          row; a rebind changes the session only when the row still holds the session and serial the caller
//          saw; rows are pruned one hour after the token's exp.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
)

//go:embed migrations/node-token/*.sql
var tokenMigrations embed.FS

func init() {
	if err := Register("node-token", tokenMigrations); err != nil {
		panic(err)
	}
}

// TokenPruneGrace is how long a bound row outlives its token.
const TokenPruneGrace = 3600

// TokenBinding is one jti bound to the certificate and session that first used it.
type TokenBinding struct {
	JTI, Serial, Session, NodeID string
	BoundAt, Exp                 int64
}

// TokenBind binds jti unless it is bound already. It returns the stored row and whether this call wrote it.
func (s *Store) TokenBind(ctx context.Context, b TokenBinding) (stored TokenBinding, created bool, err error) {
	if b.JTI == "" || b.Serial == "" || b.Session == "" || b.NodeID == "" {
		return TokenBinding{}, false, coded("E607", "a token binding needs a jti, serial, session and node")
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO node_token(jti,serial,session,node_id,bound_at,exp) VALUES(?,?,?,?,?,?)`, b.JTI, b.Serial, b.Session, b.NodeID, s.now().Unix(), b.Exp)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		created = n == 1
		return tx.QueryRowContext(ctx, `SELECT jti,serial,session,node_id,bound_at,exp FROM node_token WHERE jti=?`, b.JTI).Scan(&stored.JTI, &stored.Serial, &stored.Session, &stored.NodeID, &stored.BoundAt, &stored.Exp)
	})
	return stored, created && err == nil, err
}

// TokenCheck returns the binding of jti; found is false when the token was never used.
func (s *Store) TokenCheck(ctx context.Context, jti string) (b TokenBinding, found bool, err error) {
	err = s.readers.QueryRowContext(ctx, `SELECT jti,serial,session,node_id,bound_at,exp FROM node_token WHERE jti=?`, jti).Scan(&b.JTI, &b.Serial, &b.Session, &b.NodeID, &b.BoundAt, &b.Exp)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenBinding{}, false, nil
	}
	return b, err == nil, err
}

// TokenRebind moves jti from session `from` to session `to` when the row still names that serial and session.
// It reports whether the row moved.
func (s *Store) TokenRebind(ctx context.Context, jti, serial, from, to string) (moved bool, err error) {
	if to == "" {
		return false, coded("E607", "a rebind needs a session")
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE node_token SET session=? WHERE jti=? AND serial=? AND session=?`, to, jti, serial, from)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		moved = n == 1
		return nil
	})
	return moved && err == nil, err
}

// TokenPrune removes the rows whose token expired more than TokenPruneGrace seconds before nowUnix.
func (s *Store) TokenPrune(ctx context.Context, nowUnix int64) (removed int64, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `DELETE FROM node_token WHERE exp<?`, nowUnix-TokenPruneGrace)
		if e != nil {
			return e
		}
		removed, _ = res.RowsAffected()
		return nil
	})
	return removed, err
}
