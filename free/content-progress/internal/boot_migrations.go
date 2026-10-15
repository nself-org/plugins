// Boot-time migrations (ADR 0010, contract plugin.boot-migrations v1). Hand-wired: the plugin opens its pool
// through sdk.ConnectDB, which scripts/codemods/boot-migrations.sh does not recognise.
package internal

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"

	"github.com/nself-org/nself-content-progress/migrations"
)

func bootOptions() migrate.Options {
	return migrate.Options{Schema: "np_content_progress", FS: migrations.FS}
}

// MustBootMigrations applies the plugin's own migrations before it serves; a failure is fatal.
func MustBootMigrations(ctx context.Context, pool *pgxpool.Pool) {
	res, err := migrate.Apply(ctx, pool, bootOptions())
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations: applied %d", len(res.Applied))
}

// BootHealthHandler serves /health with the "migrations" member (applied and expected counts).
// A Status error answers 503 (the cli retries any non-200; a 200 with -1 counts is E119).
func BootHealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := migrate.Status(r.Context(), pool, bootOptions())
		code := http.StatusOK
		mig := map[string]int{"applied": -1, "expected": -1}
		if err != nil {
			log.Printf("migrations: status: %v", err)
			code = http.StatusServiceUnavailable
		} else {
			mig = map[string]int{"applied": st.Applied, "expected": st.Expected}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "plugin": "content-progress", "migrations": mig})
	}
}
