// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). The plugin talks to Postgres through
// database/sql (lib/pq); the migration helper needs a pgx pool, so a small one is opened from the same URL
// and kept for the /health migrations member.
package internal

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/plugin-clawde/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_plugin_clawde", FS: migrations.FS, StatementTimeout: 30 * time.Minute}
}

// MustBootMigrations opens the migration pool from dbURL and applies the plugin's own migrations before it
// serves; any failure is fatal. The caller closes the returned pool on shutdown.
func MustBootMigrations(ctx context.Context, dbURL string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
	return pool
}

// bootMigrationsHealth is the "migrations" member of /health: applied and expected counts.
// On a Status error it logs, answers 503 (the cli retries any non-200; a 200 with -1 counts is E119,
// a contract violation) and returns -1 counts for humans. Call it before the handler writes a header.
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}
