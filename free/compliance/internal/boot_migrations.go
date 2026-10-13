// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package internal

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-compliance/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_compliance", FS: migrations.FS}
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

// bootMigrationsHealth is the "migrations" member of /health (applied and expected counts) and the HTTP status the
// handler must answer with: 200, or 503 when the ledger cannot be read (the cli retries any non-200; a 200 with -1
// counts is E119, a contract violation).
func bootMigrationsHealth(ctx context.Context) (map[string]int, int) {
	if bootPool == nil {
		return map[string]int{"applied": -1, "expected": -1}, http.StatusServiceUnavailable
	}
	st, err := migrate.Status(ctx, bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		return map[string]int{"applied": -1, "expected": -1}, http.StatusServiceUnavailable
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}, http.StatusOK
}
