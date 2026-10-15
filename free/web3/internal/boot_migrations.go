// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package internal

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-web3/migrations"
)

var bootPool *pgxpool.Pool

// migrationStatus is migrate.Status; tests replace it (no database in the handler tests).
var migrationStatus = migrate.Status

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_web3", FS: migrations.FS}
}

// BootMigrations applies the plugin's own migrations; main calls it once, right after the pool is created.
func BootMigrations(ctx context.Context, pool *pgxpool.Pool) { mustBootMigrations(ctx, pool) }

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
	if bootPool == nil { // no pool yet (handler tests): never a panic, never a false 200
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	st, err := migrationStatus(ctx, bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}

// writeBootHealth answers GET /health. bootMigrationsHealth writes 503 itself when the status cannot be read, so the
// content type is set first and the body is encoded directly (writeJSON would send a second status line).
func writeBootHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	m := bootMigrationsHealth(r.Context(), w)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "plugin": "web3", "migrations": m})
}
