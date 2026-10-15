// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). The plugin's own migrations/*.sql are applied
// by sdk/go/migrate before the server starts; Migrate (the older embedded runner in db.go) still applies the
// internal migrations after it, and both sets are idempotent (CREATE ... IF NOT EXISTS).
package db

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-payments/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	// Index builds over existing rows can outlast the 60s default per file; a timeout would be a fatal restart loop.
	return migrate.Options{Schema: "np_payments", FS: migrations.FS, StatementTimeout: 30 * time.Minute}
}

// BootMigrations applies migrations/*.sql and logs "migrations: applied N"; main treats an error as fatal.
func BootMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		return err
	}
	log.Printf("migrations: applied %d", len(res.Applied))
	return nil
}

// BootMigrationsStatus is the "migrations" member of /health: applied and expected counts. ok is false when
// the status query fails; the handler then answers 503 (a 200 with -1 counts is a contract violation).
func BootMigrationsStatus(ctx context.Context) (counts map[string]int, ok bool) {
	if bootPool == nil {
		return map[string]int{"applied": -1, "expected": -1}, false
	}
	st, err := migrate.Status(ctx, bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		return map[string]int{"applied": -1, "expected": -1}, false
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}, true
}
