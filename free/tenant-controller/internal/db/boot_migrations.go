// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Written by
// scripts/codemods/boot-migrations.sh; edit by hand if the plugin needs tolerant mode.
package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

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
		if !strings.EqualFold(os.Getenv("NSELF_CONTROLLER_ADOPT_LEGACY"), "true") {
			return legacyLedgerError(err)
		}
		// Record 001 only (all the old runner ever applied), then let Apply run 002 and 003.
		opts := bootOptions()
		opts.FS = legacyAdoptionFS()
		if _, berr := migrate.Baseline(ctx, pool, opts); berr != nil {
			return fmt.Errorf("migrations: adopt legacy ledger: %w", berr)
		}
		log.Printf("migrations: adopted legacy ledger (recorded 001)")
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

// legacyLedgerError wraps ErrLegacyLedger with the way out, so the boot log says what to do.
func legacyLedgerError(err error) error {
	return fmt.Errorf("migrations: %w: this install predates manifest v2 and its ledger only knows migration 001; "+
		"restart once with NSELF_CONTROLLER_ADOPT_LEGACY=true to record 001 and apply the rest", err)
}

// legacyAdoptionFS exposes only migration 001: the pre-v2 runner applied nothing else, so adopting the
// ledger must not mark 002 (metering) or 003 (rename) as applied.
func legacyAdoptionFS() fs.FS {
	return only001{migrations.FS}
}

type only001 struct{ fs.FS }

func (o only001) ReadDir(name string) ([]fs.DirEntry, error) {
	ents, err := fs.ReadDir(o.FS, name)
	if err != nil {
		return nil, err
	}
	out := ents[:0]
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "001_") {
			out = append(out, e)
		}
	}
	return out, nil
}
