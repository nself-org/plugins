// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Same shape as the one written by
// scripts/codemods/boot-migrations.sh, hand-placed because the Go module root is the plugin root (go:embed
// cannot cross a module boundary) and /health lives in this package.
package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/plugins/free/job-queue/migrations"
)

var bootPool *pgxpool.Pool

// bootLog writes the plain-text boot lines (`migrations: applied N`) the boot kit and operators grep for; main installs a
// JSON slog default, which would wrap the standard logger output.
var bootLog = log.New(os.Stderr, "", log.LstdFlags)

func bootOptions() migrate.Options {
	// Index builds over existing rows can outlast the 60s default per file; a timeout would be a fatal restart loop.
	return migrate.Options{Schema: "np_job_queue", FS: migrations.FS, StatementTimeout: 30 * time.Minute}
}

// bootMigrationsHealth is the "migrations" member of /health: applied and expected counts.
// On a Status error it logs, answers 503 (the cli retries any non-200; a 200 with -1 counts is E119,
// a contract violation) and returns -1 counts for humans. Call it before the handler writes a header.
func bootMigrationsHealth(w http.ResponseWriter) map[string]int {
	// A hung database must fail the probe, not hang it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := migrate.Status(ctx, bootPool, bootOptions())
	if err != nil {
		bootLog.Printf("migrations: status: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return map[string]int{"applied": -1, "expected": -1}
	}
	return map[string]int{"applied": st.Applied, "expected": st.Expected}
}

// BootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
// main calls it once, right after the pool is created.
func BootMigrations(ctx context.Context, pool *pgxpool.Pool) {
	bootPool = pool
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		bootLog.Fatalf("migrations: %v", err)
	}
	bootLog.Printf("migrations: applied %d", len(res.Applied))
}
