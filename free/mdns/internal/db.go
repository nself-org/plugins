package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with mDNS table operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB wrapper with source_account_id defaulting to "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to a specific source_account_id.
func (d *DB) ForSourceAccount(id string) *DB {
	return &DB{pool: d.pool, sourceAccountID: id}
}

// InitSchema creates tables and indexes if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_mdns_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_name VARCHAR(255) NOT NULL,
    service_type VARCHAR(128) NOT NULL DEFAULT '_ntv._tcp',
    port INTEGER NOT NULL,
    host VARCHAR(255) NOT NULL DEFAULT 'localhost',
    domain VARCHAR(128) NOT NULL DEFAULT 'local',
    txt_records JSONB DEFAULT '{}',
    is_advertised BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, service_name, service_type)
);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_account ON np_mdns_services(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_type ON np_mdns_services(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_active ON np_mdns_services(is_active);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_advertised ON np_mdns_services(is_advertised);

CREATE TABLE IF NOT EXISTS np_mdns_discovery_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_type VARCHAR(128) NOT NULL,
    service_name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    addresses TEXT[] DEFAULT '{}',
    txt_records JSONB DEFAULT '{}',
    discovered_at TIMESTAMPTZ DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    is_available BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}',
    UNIQUE(source_account_id, service_name, service_type, host)
);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_account ON np_mdns_discovery_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_type ON np_mdns_discovery_log(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_available ON np_mdns_discovery_log(is_available);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// Ping checks database connectivity.
func (d *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.pool.Ping(ctx)
}

// --- Service CRUD ---

// CreateService inserts a new mDNS service record.
