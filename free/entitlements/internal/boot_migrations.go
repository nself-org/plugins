// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package internal

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-entitlements/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	// SearchPath keeps public on the pinned path: the migrations qualify every object with public.
	return migrate.Options{Schema: "np_entitlements", FS: migrations.FS, SearchPath: []string{"public"}, StatementTimeout: 30 * time.Minute}
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
// On a Status error it logs, answers 503 (the cli retries any non-200; a 200 with -1 counts is E119,
// a contract violation) and returns -1 counts for humans. Call it before the handler writes a header.
func bootMigrationsHealth(ctx context.Context, w http.ResponseWriter) map[string]int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if bootPool == nil { // handlers built without mustBootMigrations (unit tests); the server always sets the pool before serving
		return nil
	}
	st, err := migrate.Status(ctx, bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}
