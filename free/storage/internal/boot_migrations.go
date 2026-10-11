// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh (hand-placed in internal/ because /health lives here); edit by hand if the plugin needs tolerant mode.
package internal

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/plugin-storage/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_storage", FS: migrations.FS}
}

// mustBootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
func mustBootMigrations(ctx context.Context, pool *pgxpool.Pool) {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
}

// bootMigrationsHealth is the "migrations" member of /health: applied and expected counts.
func bootMigrationsHealth() map[string]int {
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}

// BootMigrations applies the plugin's own migrations; main calls it once, right after the pool is created.
func BootMigrations(ctx context.Context, pool *pgxpool.Pool) { mustBootMigrations(ctx, pool) }
