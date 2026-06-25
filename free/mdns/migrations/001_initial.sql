-- Migration: 001_initial
-- Plugin: mdns
-- Description: Creates np_mdns_services and np_mdns_discovery_log tables.
-- Both tables have source_account_id per Multi-Tenant Convention Wall.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS np_mdns_services (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_name      VARCHAR(255) NOT NULL,
    service_type      VARCHAR(128) NOT NULL DEFAULT '_ntv._tcp',
    port              INTEGER      NOT NULL,
    host              VARCHAR(255) NOT NULL DEFAULT 'localhost',
    domain            VARCHAR(128) NOT NULL DEFAULT 'local',
    txt_records       JSONB        DEFAULT '{}',
    is_advertised     BOOLEAN      DEFAULT false,
    is_active         BOOLEAN      DEFAULT true,
    last_seen_at      TIMESTAMPTZ  DEFAULT NOW(),
    metadata          JSONB        DEFAULT '{}',
    created_at        TIMESTAMPTZ  DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  DEFAULT NOW(),
    UNIQUE(source_account_id, service_name, service_type)
);

CREATE INDEX IF NOT EXISTS idx_np_mdns_services_account
    ON np_mdns_services(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_type
    ON np_mdns_services(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_active
    ON np_mdns_services(is_active);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_advertised
    ON np_mdns_services(is_advertised);

CREATE TABLE IF NOT EXISTS np_mdns_discovery_log (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_type      VARCHAR(128) NOT NULL,
    service_name      VARCHAR(255) NOT NULL,
    host              VARCHAR(255) NOT NULL,
    port              INTEGER      NOT NULL,
    addresses         TEXT[]       DEFAULT '{}',
    txt_records       JSONB        DEFAULT '{}',
    discovered_at     TIMESTAMPTZ  DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ  DEFAULT NOW(),
    is_available      BOOLEAN      DEFAULT true,
    metadata          JSONB        DEFAULT '{}',
    UNIQUE(source_account_id, service_name, service_type, host)
);

CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_account
    ON np_mdns_discovery_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_type
    ON np_mdns_discovery_log(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_available
    ON np_mdns_discovery_log(is_available);
