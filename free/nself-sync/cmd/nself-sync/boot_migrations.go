// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh; the /health handler lives here because it reports the ledger counts.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-sync/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	// The materialized view build can outlast the 60s default per file on a populated install.
	return migrate.Options{Schema: "np_nself_sync", FS: migrations.FS, StatementTimeout: 30 * time.Minute}
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

// handleHealth answers GET /health with the "migrations" member: applied and expected counts.
// A Status error answers 503 (the cli retries any non-200; a 200 with -1 counts is a contract violation).
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"status": "ok"}
	if bootPool != nil {
		// A hung database must fail the probe, not hang it.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := migrate.Status(ctx, bootPool, bootOptions())
		if err != nil {
			log.Printf("migrations: status: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			body["migrations"] = map[string]int{"applied": st.Applied, "expected": st.Expected}
		}
	}
	_ = json.NewEncoder(w).Encode(body)
}
