// seed — local-first dev seeding for the ɳSentry SaaS stack (W8 preset).
//
// Purpose: create the deterministic dev tenant + `nsk_dev_local_...` API key
//
//	so `nself start` (sentry preset) gives a working authenticated API with
//	zero manual steps. Idempotent: safe to run on every boot.
//
// Usage:   DATABASE_URL=postgres://... go run ./cmd/seed
//
//	Flags:  -tier (default plus — dev should exercise every feature)
//	        -email (default dev@localhost)
//
// Outputs: prints the tenant id and the raw dev key.
// Constraints: the dev key is DETERMINISTIC and therefore known — never run
//
//	this against a production database (the tool refuses non-local hosts
//	unless -force is passed).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// Deterministic dev identifiers (W8 local-first preset contract).
const (
	// DevTenantID is the fixed dev tenant UUID.
	DevTenantID = "00000000-0000-4000-8000-000000000001"
	// DevAPIKey is nsk_ + 64 chars ("dev_local_" + 54 zeros) — satisfies the
	// nsk_<64> format check in saas.AuthenticateKey while staying readable.
	DevAPIKey = "nsk_dev_local_000000000000000000000000000000000000000000000000000000"
)

func isLocalDatabase(dbURL string) bool {
	u, err := url.Parse(dbURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		host == "postgres" || strings.HasSuffix(host, ".local")
}

func main() {
	tier := flag.String("tier", string(saas.TierPlus), "dev tenant tier (free|bundle|plus)")
	email := flag.String("email", "dev@localhost", "dev tenant owner email")
	force := flag.Bool("force", false, "allow seeding a non-local database")
	flag.Parse()

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Fatal("seed: DATABASE_URL is required")
	}
	if !isLocalDatabase(dbURL) && !*force {
		log.Fatal("seed: refusing to seed a non-local database (the dev key is deterministic); pass -force to override")
	}
	switch saas.Tier(*tier) {
	case saas.TierFree, saas.TierBundle, saas.TierPlus:
	default:
		log.Fatalf("seed: invalid tier %q", *tier)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("seed: open database: %v", err)
	}
	defer db.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := saas.EnsureSchema(ctx, db); err != nil {
		log.Fatalf("seed: ensure schema: %v", err)
	}

	// Tenant upsert (tier refresh keeps re-seeds honest).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO np_saas_tenants (tenant_id, tier, owner_email)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id) DO UPDATE SET
			tier = EXCLUDED.tier, owner_email = EXCLUDED.owner_email, updated_at = NOW()`,
		DevTenantID, *tier, *email); err != nil {
		log.Fatalf("seed: upsert dev tenant: %v", err)
	}

	// Deterministic dev key (hash-stored, idempotent).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO np_saas_api_keys (tenant_id, key_hash, key_prefix, name, scopes)
		VALUES ($1, $2, $3, 'dev-local', 'full')
		ON CONFLICT (key_hash) DO NOTHING`,
		DevTenantID, saas.HashKey(DevAPIKey), saas.DisplayPrefix(DevAPIKey)); err != nil {
		log.Fatalf("seed: insert dev api key: %v", err)
	}

	fmt.Printf("dev tenant: %s (tier %s, %s)\n", DevTenantID, *tier, *email)
	fmt.Printf("dev api key: %s\n", DevAPIKey)
	fmt.Println("export NSELF_SENTRY_API_KEY=" + DevAPIKey)
}
