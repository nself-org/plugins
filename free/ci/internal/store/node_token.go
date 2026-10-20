package store

// Purpose: the durable first-use binding of a job token (P7-NODE-08, migration set node-token): the token's jti is
//          bound to the certificate serial and the session that first presented it, so a replay from another
//          certificate or session is refused even after a coordinator restart.
// Inputs:  jti, certificate serial, session id, node id, token exp (unix seconds).
// Outputs: TokenBind (first writer wins, the stored row comes back), TokenCheck, TokenRebind (compare and swap
//          of the session), TokenSpend (the job's result was accepted: the token is used up), TokenPrune.
// Constraints: the table holds ids and times only, never a token; one jti has one row; a bind never replaces a
//          row; a rebind changes the session (and the serial after a renewal) only when the row still holds the
//          serial and session the caller saw; rows are pruned one hour after the token's exp.

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

// TokenPruneGrace is how long a bound row outlives its token, in seconds.
const TokenPruneGrace = 3600

// TokenBinding is one jti bound to the certificate and session that first used it.
type TokenBinding struct {
	JTI, Serial, Session, NodeID string
	BoundAt, Exp                 int64
	Spent                        bool // the job's result was accepted; the token is used up
}

const jobTokenCols = `jti,serial,session,node_id,bound_at,exp,spent`

type rowScanner interface{ Scan(...any) error }

func scanBinding(r rowScanner) (b TokenBinding, err error) {
	var spent int
	if err = r.Scan(&b.JTI, &b.Serial, &b.Session, &b.NodeID, &b.BoundAt, &b.Exp, &spent); err == nil {
		b.Spent = spent != 0
	}
	return b, err
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
		stored, e = scanBinding(tx.QueryRowContext(ctx, `SELECT `+jobTokenCols+` FROM node_token WHERE jti=?`, b.JTI))
		return e
	})
	return stored, created && err == nil, err
}

// TokenCheck returns the binding of jti; found is false when the token was never used.
func (s *Store) TokenCheck(ctx context.Context, jti string) (b TokenBinding, found bool, err error) {
	b, err = scanBinding(s.readers.QueryRowContext(ctx, `SELECT `+jobTokenCols+` FROM node_token WHERE jti=?`, jti))
	if errors.Is(err, sql.ErrNoRows) {
		return TokenBinding{}, false, nil
	}
	return b, err == nil, err
}

// TokenRebind moves jti from (fromSerial, fromSession) to (toSerial, toSession) when the row still names exactly
// that serial and session and the token is not spent. It reports whether the row moved. The serial changes when the node renewed its
// certificate between two sessions.
func (s *Store) TokenRebind(ctx context.Context, jti, fromSerial, fromSession, toSerial, toSession string) (moved bool, err error) {
	if toSerial == "" || toSession == "" {
		return false, coded("E607", "a rebind needs a serial and a session")
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE node_token SET serial=?, session=? WHERE jti=? AND serial=? AND session=? AND spent=0`, toSerial, toSession, jti, fromSerial, fromSession)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		moved = n == 1
		return nil
	})
	return moved && err == nil, err
}

// TokenSpend marks jti used up: the result of its job was accepted. A token that was never bound stays unknown.
func (s *Store) TokenSpend(ctx context.Context, jti string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE node_token SET spent=1 WHERE jti=?`, jti)
		return err
	})
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
