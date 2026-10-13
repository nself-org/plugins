// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh; the /health handler lives here because it reports the ledger counts.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-vault/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_nself_vault", FS: migrations.FS}
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
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		body["migrations"] = map[string]int{"applied": st.Applied, "expected": st.Expected}
	}
	_ = json.NewEncoder(w).Encode(body)
}
