// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh (hand-placed in internal/ because /health lives here); edit by hand if the plugin needs tolerant mode.
package internal

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-feature-flags/migrations"
)

var bootPool *pgxpool.Pool

// legacyTable pairs a table an older release named without the np_ prefix with the name 001 renames it to.
type legacyTable struct{ old, renamed string }

// legacyTables are the renames migration 001 guards. A survivor means both names existed at upgrade time.
var legacyTables = []legacyTable{
	{"ff_flags", "np_flags_flags"},
	{"ff_rules", "np_flags_rules"},
	{"ff_segments", "np_flags_segments"},
	{"ff_evaluations", "np_flags_evaluations"},
	{"ff_webhook_events", "np_flags_webhook_events"},
}

func bootOptions() migrate.Options {
	// Index builds over existing rows can outlast the 60s default per file; a timeout would be a fatal restart loop.
	// SearchPath keeps public on the pinned path: the migrations qualify every object with public.
	return migrate.Options{Schema: "np_feature_flags", FS: migrations.FS, SearchPath: []string{"public"}, StatementTimeout: 30 * time.Minute}
}

// mustBootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
func mustBootMigrations(ctx context.Context, pool *pgxpool.Pool) {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
	reportLegacyTables(ctx, pool)
}

// reportLegacyTables logs each old-name table that is still present: migration 001 renames only when the new name
// is free, so a survivor existed beside its np_ table and was left untouched (nothing merged or dropped). SELECTs only.
func reportLegacyTables(ctx context.Context, pool *pgxpool.Pool) {
	for _, t := range legacyTables {
		var present bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+t.old).Scan(&present); err != nil {
			log.Printf("migrations: cannot check legacy table public.%s: %v", t.old, err)
			continue
		}
		if present {
			log.Printf("migrations: legacy table public.%s kept beside public.%s: both existed, nothing merged or dropped (see README)", t.old, t.renamed)
		}
	}
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
