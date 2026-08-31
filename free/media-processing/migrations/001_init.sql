-- plugin-media-processing: Media encoding schema
-- Convention Wall: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Columns derived from handlers.rs SELECT/INSERT statements (code wins)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_mediap_encoding_profiles (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    name              TEXT        NOT NULL,
    description       TEXT,
    is_default        BOOLEAN     NOT NULL DEFAULT FALSE,
    video_codec       TEXT        NOT NULL DEFAULT 'h264',
    audio_codec       TEXT        NOT NULL DEFAULT 'aac',
    container         TEXT        NOT NULL DEFAULT 'mp4',
    bitrate_kbps      INTEGER,
    width             INTEGER,
    height            INTEGER,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_mediap_encoding_profiles_source_account
    ON np_mediap_encoding_profiles(source_account_id);

CREATE TABLE IF NOT EXISTS np_mediap_jobs (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    profile_id        UUID        REFERENCES np_mediap_encoding_profiles(id) ON DELETE SET NULL,
    input_url         TEXT        NOT NULL,
    output_path       TEXT,
    status            TEXT        NOT NULL DEFAULT 'pending',
    priority          INTEGER     NOT NULL DEFAULT 0,
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_mediap_jobs_source_account ON np_mediap_jobs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mediap_jobs_status ON np_mediap_jobs(status);

CREATE TABLE IF NOT EXISTS np_mediap_job_outputs (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID        NOT NULL REFERENCES np_mediap_jobs(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    output_type       TEXT        NOT NULL,
    url               TEXT,
    size_bytes        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_mediap_job_outputs_source_account ON np_mediap_job_outputs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mediap_job_outputs_job_id ON np_mediap_job_outputs(job_id);

CREATE TABLE IF NOT EXISTS np_mediap_hls_manifests (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID        NOT NULL REFERENCES np_mediap_jobs(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    manifest_url      TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_mediap_hls_manifests_source_account ON np_mediap_hls_manifests(source_account_id);

CREATE TABLE IF NOT EXISTS np_mediap_subtitles (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID        NOT NULL REFERENCES np_mediap_jobs(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    language          TEXT        NOT NULL DEFAULT 'en',
    subtitle_url      TEXT        NOT NULL,
    format            TEXT        NOT NULL DEFAULT 'vtt',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_mediap_subtitles_source_account ON np_mediap_subtitles(source_account_id);
