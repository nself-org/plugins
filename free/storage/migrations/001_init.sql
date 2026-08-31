-- plugin-storage: initial schema
-- port 9007 | Multi-App Isolation: source_account_id on all tables

CREATE TABLE IF NOT EXISTS np_storage_buckets (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    name              TEXT        NOT NULL,
    region            TEXT        NOT NULL DEFAULT 'us-east-1',
    versioning        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, name)
);

CREATE INDEX IF NOT EXISTS idx_np_storage_buckets_source
    ON np_storage_buckets (source_account_id);

CREATE TABLE IF NOT EXISTS np_storage_objects (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    bucket_id         UUID        NOT NULL REFERENCES np_storage_buckets(id) ON DELETE CASCADE,
    key               TEXT        NOT NULL,
    size_bytes        BIGINT      NOT NULL DEFAULT 0,
    content_type      TEXT        NOT NULL DEFAULT 'application/octet-stream',
    etag              TEXT,
    metadata          JSONB       NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (bucket_id, key)
);

CREATE INDEX IF NOT EXISTS idx_np_storage_objects_bucket
    ON np_storage_objects (bucket_id);
CREATE INDEX IF NOT EXISTS idx_np_storage_objects_source
    ON np_storage_objects (source_account_id);

CREATE TABLE IF NOT EXISTS np_storage_metadata (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    object_id         UUID        NOT NULL REFERENCES np_storage_objects(id) ON DELETE CASCADE,
    key               TEXT        NOT NULL,
    value             TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (object_id, key)
);

CREATE INDEX IF NOT EXISTS idx_np_storage_metadata_object
    ON np_storage_metadata (object_id);
