package tenantdb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nself-org/nself-byok/tenantdb"
	"github.com/nself-org/nself-byok/tenantdb/tenantdbtest"
)

// TestWith drives tenantdb.With through its outcomes: the tenant setting is the first
// statement of the transaction and is transaction-local, a failing fn rolls back, and a
// zero tenant never reaches the database.
func TestWith(t *testing.T) {
	tenant := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	boom := errors.New("boom")
	tests := []struct {
		name         string
		tenant       uuid.UUID
		fn           func(tx pgx.Tx) error
		wantErr      error
		wantBegins   int
		wantCommits  int
		wantSQLCalls []string
	}{
		{
			name:   "sets the tenant before the work and commits",
			tenant: tenant,
			fn: func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), "SELECT 1")
				return err
			},
			wantBegins: 1, wantCommits: 1,
			wantSQLCalls: []string{"SELECT set_config($1, $2, true)", "SELECT 1"},
		},
		{
			name:    "fn error rolls back and does not commit",
			tenant:  tenant,
			fn:      func(pgx.Tx) error { return boom },
			wantErr: boom, wantBegins: 1, wantCommits: 0,
			wantSQLCalls: []string{"SELECT set_config($1, $2, true)"},
		},
		{
			name:    "zero tenant is refused before any transaction",
			tenant:  uuid.Nil,
			fn:      func(pgx.Tx) error { t.Fatal("fn must not run"); return nil },
			wantErr: tenantdb.ErrNoTenant,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := &tenantdbtest.Pool{}
			err := tenantdb.With(context.Background(), pool, tc.tenant, tc.fn)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if pool.Begins != tc.wantBegins || pool.Commits != tc.wantCommits {
				t.Fatalf("begins/commits = %d/%d, want %d/%d", pool.Begins, pool.Commits, tc.wantBegins, tc.wantCommits)
			}
			if tc.wantBegins > 0 && pool.Rollbacks != 1 {
				t.Fatalf("rollbacks = %d, want 1 (deferred, harmless after commit)", pool.Rollbacks)
			}
			var got []string
			for _, c := range pool.Calls {
				got = append(got, c.SQL)
			}
			if len(got) != len(tc.wantSQLCalls) {
				t.Fatalf("statements = %q, want %q", got, tc.wantSQLCalls)
			}
			for i := range got {
				if got[i] != tc.wantSQLCalls[i] {
					t.Fatalf("statement %d = %q, want %q", i, got[i], tc.wantSQLCalls[i])
				}
			}
			if len(pool.Calls) > 0 {
				c := pool.Calls[0]
				if c.Args[0] != tenantdb.Setting || c.Args[1] != tc.tenant.String() {
					t.Fatalf("set_config args = %v, want [%s %s]", c.Args, tenantdb.Setting, tc.tenant)
				}
			}
		})
	}
}
