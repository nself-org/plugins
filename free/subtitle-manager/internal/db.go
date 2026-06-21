package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with subtitle-manager table operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// InitSchema creates all tables and indexes if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_subtmgr_subtitles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    media_id VARCHAR(255) NOT NULL,
    media_type VARCHAR(50) NOT NULL,
    language VARCHAR(10) NOT NULL,
    file_path TEXT NOT NULL,
    source VARCHAR(50) NOT NULL,
    sync_score DECIMAL(5,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_subtitles_media
    ON np_subtmgr_subtitles(media_id, language);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_subtitles_account
    ON np_subtmgr_subtitles(source_account_id);

CREATE TABLE IF NOT EXISTS np_subtmgr_downloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    subtitle_id UUID REFERENCES np_subtmgr_subtitles(id) ON DELETE CASCADE,
    media_id VARCHAR(255) NOT NULL,
    media_type VARCHAR(50) NOT NULL,
    media_title VARCHAR(255),
    language VARCHAR(10) NOT NULL,
    file_path TEXT NOT NULL,
    file_size_bytes BIGINT,
    opensubtitles_file_id INT,
    file_hash VARCHAR(64),
    sync_score DECIMAL(5,2),
    source VARCHAR(50) NOT NULL DEFAULT 'opensubtitles',
    qc_status VARCHAR(20),
    qc_details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_downloads_media
    ON np_subtmgr_downloads(media_id, language);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_downloads_account
    ON np_subtmgr_downloads(source_account_id);

CREATE TABLE IF NOT EXISTS np_subtmgr_qc_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    download_id UUID REFERENCES np_subtmgr_downloads(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,
    checks JSONB NOT NULL DEFAULT '[]',
    issues JSONB NOT NULL DEFAULT '[]',
    cue_count INT NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_qc_results_download
    ON np_subtmgr_qc_results(download_id);

CREATE INDEX IF NOT EXISTS idx_np_subtmgr_qc_results_account
    ON np_subtmgr_qc_results(source_account_id);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// ---------------------------------------------------------------------------
// Subtitles CRUD
// ---------------------------------------------------------------------------

// SearchSubtitles finds locally stored subtitles for a given media and language.
