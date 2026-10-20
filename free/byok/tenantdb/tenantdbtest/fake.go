// Package tenantdbtest is a recording stand-in for a pgx pool, so byok code that talks to
// the database under tenantdb.With can be tested without Postgres. It is test support only.
package tenantdbtest

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Call is one statement the code under test sent.
type Call struct {
	TxID int
	SQL  string
	Args []any
}

// Pool records every statement per transaction. Rows are never returned: QueryRow yields
// pgx.ErrNoRows and Query yields an error, which is enough for the code paths the tests
// drive (they only assert what was sent and in which order).
type Pool struct {
	mu        sync.Mutex
	Calls     []Call
	Begins    int
	Commits   int
	Rollbacks int
}

// Begin starts a recorded transaction.
func (p *Pool) Begin(_ context.Context) (pgx.Tx, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Begins++
	return &tx{p: p, id: p.Begins}, nil
}

type tx struct {
	pgx.Tx // unused methods panic on the nil embedded interface; the tests never call them
	p      *Pool
	id     int
}

func (t *tx) record(sql string, args []any) {
	t.p.mu.Lock()
	defer t.p.mu.Unlock()
	t.p.Calls = append(t.p.Calls, Call{TxID: t.id, SQL: sql, Args: args})
}

func (t *tx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.record(sql, args)
	return pgconn.CommandTag{}, nil
}

func (t *tx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	t.record(sql, args)
	return noRow{}
}

func (t *tx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	t.record(sql, args)
	return nil, pgx.ErrNoRows
}

func (t *tx) Commit(context.Context) error {
	t.p.mu.Lock()
	defer t.p.mu.Unlock()
	t.p.Commits++
	return nil
}

func (t *tx) Rollback(context.Context) error {
	t.p.mu.Lock()
	defer t.p.mu.Unlock()
	t.p.Rollbacks++
	return nil
}

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }
