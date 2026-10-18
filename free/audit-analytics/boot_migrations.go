// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1).
// Purpose: apply migrations/*.sql through sdk/go/migrate before the plugin serves, and report counts for /health.
// The plugin queries through database/sql (lib/pq); migrate needs a pgx pool, so one small pool is kept for it.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-audit-analytics/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	// SearchPath keeps public on the pinned path: the migrations qualify every object with public.
	return migrate.Options{Schema: "np_audit_analytics", FS: migrations.FS, SearchPath: []string{"public"}, StatementTimeout: 30 * time.Minute}
}

// mustBootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
func mustBootMigrations(ctx context.Context, dbURL string) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("audit-analytics: migrations: connect: %v", err)
	}
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("audit-analytics: migrations: %v", err)
	}
	log.Printf("audit-analytics: migrations: applied %d", len(res.Applied))
}

// bootMigrationsHealth is the "migrations" member of /health: applied and expected counts.
// On a Status error it logs, answers 503 (the cli retries any non-200) and returns -1 counts for humans.
// Call it before the handler writes a header.
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	if bootPool == nil { // handlers built without mustBootMigrations (unit tests)
		return nil
	}
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("audit-analytics: migrations: status: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}
