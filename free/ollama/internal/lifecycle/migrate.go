// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1) for the lifecycle binary.
// The binary has no HTTP server: it applies the plugin's own migrations when it starts or installs,
// provided DATABASE_URL is set (the model registry table is optional state, not needed to pull a model).
package lifecycle

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/plugins/free/ollama/migrations"
)

// BootMigrations applies migrations/*.sql to the np_ollama schema. With DATABASE_URL unset it logs and returns nil.
func BootMigrations(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Print("migrations: skipped, DATABASE_URL is not set")
		return nil
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("migrations: connect: %w", err)
	}
	defer pool.Close()
	// Index builds over existing rows can outlast the 60s default per file.
	res, err := migrate.Apply(ctx, pool, migrate.Options{Schema: "np_ollama", FS: migrations.FS, StatementTimeout: 30 * time.Minute})
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
	return nil
}
