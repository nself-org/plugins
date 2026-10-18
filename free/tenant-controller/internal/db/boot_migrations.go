// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-tenant-controller/migrations"
)

var bootPool *pgxpool.Pool

func bootOptions() migrate.Options {
	// SearchPath keeps public on the pinned path: 002 qualifies the np_cloud_* tables with public.
	return migrate.Options{Schema: "np_tenant_controller", FS: migrations.FS, SearchPath: []string{"public"}}
}

// applyBootMigrations applies the plugin's own migrations before it serves.
func applyBootMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if errors.Is(err, migrate.ErrLegacyLedger) {
		// A pre-manifest-v2 install: its own runner applied every migration file and kept a (filename, applied_at)
		// ledger. Adopt it once (adds and backfills the checksum column), then apply what is left.
		if _, berr := migrate.Baseline(ctx, pool, bootOptions()); berr != nil {
			return fmt.Errorf("migrations: baseline legacy ledger: %w", berr)
		}
		res, err = migrate.Apply(ctx, pool, bootOptions())
	}
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
	return nil
}

// MigrationsHealth is the "migrations" member of /health: applied and expected counts.
// On a Status error it logs, answers 503 (the cli retries any non-200; a 200 with -1 counts is E119,
// a contract violation) and returns -1 counts for humans. Call it before the handler writes a header.
func MigrationsHealth(w http.ResponseWriter) map[string]int {
	if bootPool == nil { // unit tests build handlers without a pool
		return nil
	}
	st, err := migrate.Status(context.Background(), bootPool, bootOptions())
	if err != nil {
		log.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}
