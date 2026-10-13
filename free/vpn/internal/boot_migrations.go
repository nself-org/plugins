// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh (hand-placed: this plugin builds its pool with sdk.ConnectDB and serves /health from the sdk router).
package internal

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-vpn/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_vpn", FS: migrations.FS}
}

// BootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
// main calls it once, right after the pool is created.
func BootMigrations(ctx context.Context, pool *pgxpool.Pool) {
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
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	if bootPool == nil { // no pool yet (tests): never a panic, never a false 200
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}

// BootHealthHandler serves GET /health with the plugin status and the "migrations" member. Register it on the
// sdk router before ListenAndServe: the sdk only adds its default /health when none exists.
func BootHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m := bootMigrationsHealth(w)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "plugin": "vpn", "migrations": m})
	}
}
