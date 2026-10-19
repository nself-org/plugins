package store

// Purpose: the chained view of the core audit table (Epic D20). Rows other packages write through
// appendAudit have no head; sealing walks them in seq order, sets prev_head and head, and leaves the
// signature column to the TEAM checkpoint (P7-TRUST-20).
// Inputs:  AuditInput (a row plus the chain function); the core audit table.
// Outputs: sealed rows, AuditEntry pages for verification, the unsealed count.
// Constraints: expand-only migration set trust-audit (one partial index, no new table); appending and
//          sealing share one transaction; the chain function lives in internal/audit (store never hashes);
//          detail never carries a secret (the core row check and the audit scrubber both run).

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/trust-audit/*.sql
var trustAuditMigrations embed.FS

func init() {
	if err := Register("trust-audit", trustAuditMigrations); err != nil {
		panic(err)
	}
}

// AuditInput is one row to write; a non-nil Seal also chains every unsealed row, this one last.
type AuditInput struct {
	Actor, Action, Target, Detail string
	Seal                          AuditSealer
}

// AuditEntry is a stored audit row with its chain columns.
type AuditEntry struct {
	AuditRow
	PrevHead, Head, Signature []byte
}

func (s *Store) auditIn(ctx context.Context, tx *sql.Tx, in AuditInput) error {
	if err := s.appendAudit(ctx, tx, in.Actor, in.Action, in.Target, in.Detail); err != nil {
		return err
	}
	if in.Seal == nil {
		return nil
	}
	_, err := sealPending(ctx, tx, in.Seal)
	return err
}

// busyRetry bounds how long an audit write waits for another process that holds the database lock; a
// SQLite busy timeout alone can starve one of two steady writers.
const busyRetry = 30 * time.Second

func isBusy(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked"))
}

// writeRetry runs write, repeating it (the failed attempt rolled back) while the database is busy.
func (s *Store) writeRetry(ctx context.Context, fn func(*sql.Tx) error) error {
	deadline := time.Now().Add(busyRetry)
	for wait := time.Millisecond; ; wait = min(wait*2, 50*time.Millisecond) {
		err := s.write(ctx, fn)
		if !isBusy(err) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// AuditAppend writes the row (and seals when in.Seal is set) in one transaction.
func (s *Store) AuditAppend(ctx context.Context, in AuditInput) error {
	return s.writeRetry(ctx, func(tx *sql.Tx) error { return s.auditIn(ctx, tx, in) })
}

// AuditAppend writes the row inside the caller's transaction (P7-TRUST-26 policy writes).
func (t *Tx) AuditAppend(in AuditInput) error { return t.s.auditIn(t.ctx, t.tx, in) }

// AuditSeal chains every unsealed row in seq order and returns how many it sealed.
func (s *Store) AuditSeal(ctx context.Context, seal AuditSealer) (n int, err error) {
	if seal == nil {
		return 0, coded("E607", "no chain function")
	}
	err = s.writeRetry(ctx, func(tx *sql.Tx) (e error) { n, e = sealPending(ctx, tx, seal); return e })
	return n, err
}

func sealPending(ctx context.Context, tx *sql.Tx, seal AuditSealer) (int, error) {
	rows, err := tx.QueryContext(ctx, "SELECT seq,ts,actor,action,target,detail FROM audit WHERE head IS NULL ORDER BY seq")
	if err != nil {
		return 0, err
	}
	var todo []AuditRow
	for rows.Next() {
		var r AuditRow
		if err = rows.Scan(&r.Seq, &r.TS, &r.Actor, &r.Action, &r.Target, &r.Detail); err != nil {
			_ = rows.Close()
			return 0, err
		}
		todo = append(todo, r)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil || len(todo) == 0 {
		return 0, err
	}
	// Rows are sealed in seq order under one write lock, so an unsealed row below a sealed one means a head
	// was cleared. Sealing it would spread the damage; refuse and let --verify name the row.
	var top int64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM audit WHERE head IS NOT NULL").Scan(&top); err != nil {
		return 0, err
	}
	if top > todo[0].Seq {
		return 0, coded("E607", "audit row "+strconv.FormatInt(todo[0].Seq, 10)+" is unsealed below a sealed row; run `nself ci serve admin audit --verify`")
	}
	var prev []byte
	err = tx.QueryRowContext(ctx, "SELECT head FROM audit WHERE seq<? AND head IS NOT NULL ORDER BY seq DESC LIMIT 1", todo[0].Seq).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	for _, r := range todo {
		c, err := seal(prev, r)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, "UPDATE audit SET prev_head=?,head=?,signature=? WHERE seq=? AND head IS NULL", prev, c.Head, c.Signature, r.Seq)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return 0, coded("E607", "audit row changed while sealing")
		}
		prev = c.Head
	}
	return len(todo), nil
}

type querier interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// AuditEntries returns up to limit rows with seq > after, in seq order.
func (s *Store) AuditEntries(ctx context.Context, after int64, limit int) ([]AuditEntry, error) {
	return auditEntries(ctx, s.readers, after, limit)
}

// AuditSnapshot is one read transaction over the audit table: every page it returns comes from the same
// moment, so a sealer committing between pages cannot make a verification see a half-sealed chain.
type AuditSnapshot struct{ tx *sql.Tx }

// AuditSnapshot opens a snapshot; Close it.
func (s *Store) AuditSnapshot(ctx context.Context) (*AuditSnapshot, error) {
	tx, err := s.readers.BeginTx(ctx, nil)
	return &AuditSnapshot{tx: tx}, err
}

// Entries is AuditEntries inside the snapshot.
func (a *AuditSnapshot) Entries(ctx context.Context, after int64, limit int) ([]AuditEntry, error) {
	return auditEntries(ctx, a.tx, after, limit)
}

// HighSeq is the highest seq the table ever issued (its AUTOINCREMENT counter), 0 for an empty log. Rows
// are never deleted, so a counter above the last row means the newest rows were removed.
func (a *AuditSnapshot) HighSeq(ctx context.Context) (int64, error) {
	var n int64
	err := a.tx.QueryRowContext(ctx, "SELECT seq FROM sqlite_sequence WHERE name='audit'").Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// Close ends the snapshot.
func (a *AuditSnapshot) Close() error { return a.tx.Rollback() }

func auditEntries(ctx context.Context, q querier, after int64, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := q.QueryContext(ctx, "SELECT seq,ts,actor,action,target,detail,prev_head,head,signature FROM audit WHERE seq>? ORDER BY seq LIMIT ?", after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err = rows.Scan(&e.Seq, &e.TS, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.PrevHead, &e.Head, &e.Signature); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditPending reports the unsealed row count and the timestamp (ns) of the oldest one (0 when none).
func (s *Store) AuditPending(ctx context.Context) (n int, oldestTS int64, err error) {
	var ts sql.NullInt64
	err = s.readers.QueryRowContext(ctx, "SELECT COUNT(*), MIN(ts) FROM audit WHERE head IS NULL").Scan(&n, &ts)
	return n, ts.Int64, err
}

// Now is the store clock, shared so a sealer and a verifier agree on "past the interval".
func (s *Store) Now() int64 { return s.now().UnixNano() }
