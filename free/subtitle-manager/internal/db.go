package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
func (d *DB) SearchSubtitles(mediaID, language, sourceAccountID string) ([]SubtitleRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, media_id, media_type, language, file_path,
		        source, sync_score, created_at, updated_at
		 FROM np_subtmgr_subtitles
		 WHERE media_id = $1 AND language = $2 AND source_account_id = $3
		 ORDER BY sync_score DESC NULLS LAST, updated_at DESC`,
		mediaID, language, sourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("search subtitles: %w", err)
	}
	defer rows.Close()

	var results []SubtitleRecord
	for rows.Next() {
		var s SubtitleRecord
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.MediaID, &s.MediaType,
			&s.Language, &s.FilePath, &s.Source, &s.SyncScore,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subtitle: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// UpsertSubtitle inserts or updates a subtitle record.
func (d *DB) UpsertSubtitle(data UpsertSubtitleInput) (*SubtitleRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	accountID := data.SourceAccountID
	if accountID == "" {
		accountID = "primary"
	}

	var s SubtitleRecord
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_subtmgr_subtitles
		    (source_account_id, media_id, media_type, language, file_path, source, sync_score)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO UPDATE SET
		    media_type = EXCLUDED.media_type,
		    language = EXCLUDED.language,
		    file_path = EXCLUDED.file_path,
		    source = EXCLUDED.source,
		    sync_score = EXCLUDED.sync_score,
		    updated_at = NOW()
		 RETURNING id, source_account_id, media_id, media_type, language, file_path,
		           source, sync_score, created_at, updated_at`,
		accountID, data.MediaID, data.MediaType, data.Language,
		data.FilePath, data.Source, data.SyncScore,
	).Scan(&s.ID, &s.SourceAccountID, &s.MediaID, &s.MediaType,
		&s.Language, &s.FilePath, &s.Source, &s.SyncScore,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert subtitle: %w", err)
	}
	return &s, nil
}

// ---------------------------------------------------------------------------
// Downloads CRUD
// ---------------------------------------------------------------------------

// InsertDownload inserts a new download record and returns it.
func (d *DB) InsertDownload(data InsertDownloadInput) (*DownloadRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	accountID := data.SourceAccountID
	if accountID == "" {
		accountID = "primary"
	}

	var dl DownloadRecord
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_subtmgr_downloads
		    (source_account_id, subtitle_id, media_id, media_type, media_title,
		     language, file_path, file_size_bytes, opensubtitles_file_id,
		     file_hash, sync_score, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id, source_account_id, subtitle_id, media_id, media_type, media_title,
		           language, file_path, file_size_bytes, opensubtitles_file_id,
		           file_hash, sync_score, source, qc_status, qc_details, created_at, updated_at`,
		accountID, nilIfEmpty(data.SubtitleID), data.MediaID, data.MediaType,
		nilIfEmptyStr(data.MediaTitle), data.Language, data.FilePath,
		nilIfZeroInt64(data.FileSizeBytes), nilIfZeroInt(data.OpensubtitlesFileID),
		nilIfEmptyStr(data.FileHash), data.SyncScore, data.Source,
	).Scan(&dl.ID, &dl.SourceAccountID, &dl.SubtitleID, &dl.MediaID,
		&dl.MediaType, &dl.MediaTitle, &dl.Language, &dl.FilePath,
		&dl.FileSizeBytes, &dl.OpensubtitlesFileID, &dl.FileHash,
		&dl.SyncScore, &dl.Source, &dl.QCStatus, &dl.QCDetails,
		&dl.CreatedAt, &dl.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert download: %w", err)
	}
	return &dl, nil
}

// GetDownloadByMediaID finds the most recent download for a media+language+account.
func (d *DB) GetDownloadByMediaID(mediaID, language, sourceAccountID string) (*DownloadRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dl DownloadRecord
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, subtitle_id, media_id, media_type, media_title,
		        language, file_path, file_size_bytes, opensubtitles_file_id,
		        file_hash, sync_score, source, qc_status, qc_details, created_at, updated_at
		 FROM np_subtmgr_downloads
		 WHERE media_id = $1 AND language = $2 AND source_account_id = $3
		 ORDER BY created_at DESC
		 LIMIT 1`,
		mediaID, language, sourceAccountID,
	).Scan(&dl.ID, &dl.SourceAccountID, &dl.SubtitleID, &dl.MediaID,
		&dl.MediaType, &dl.MediaTitle, &dl.Language, &dl.FilePath,
		&dl.FileSizeBytes, &dl.OpensubtitlesFileID, &dl.FileHash,
		&dl.SyncScore, &dl.Source, &dl.QCStatus, &dl.QCDetails,
		&dl.CreatedAt, &dl.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get download by media: %w", err)
	}
	return &dl, nil
}

// ListDownloads returns paginated downloads for an account.
func (d *DB) ListDownloads(sourceAccountID string, limit, offset int) ([]DownloadRecord, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var total int
	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_subtmgr_downloads WHERE source_account_id = $1`,
		sourceAccountID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count downloads: %w", err)
	}

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, subtitle_id, media_id, media_type, media_title,
		        language, file_path, file_size_bytes, opensubtitles_file_id,
		        file_hash, sync_score, source, qc_status, qc_details, created_at, updated_at
		 FROM np_subtmgr_downloads
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		sourceAccountID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list downloads: %w", err)
	}
	defer rows.Close()

	var downloads []DownloadRecord
	for rows.Next() {
		var dl DownloadRecord
		if err := rows.Scan(&dl.ID, &dl.SourceAccountID, &dl.SubtitleID, &dl.MediaID,
			&dl.MediaType, &dl.MediaTitle, &dl.Language, &dl.FilePath,
			&dl.FileSizeBytes, &dl.OpensubtitlesFileID, &dl.FileHash,
			&dl.SyncScore, &dl.Source, &dl.QCStatus, &dl.QCDetails,
			&dl.CreatedAt, &dl.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan download: %w", err)
		}
		downloads = append(downloads, dl)
	}
	return downloads, total, rows.Err()
}

// DeleteDownload removes a download by ID. Returns true if a row was deleted.
func (d *DB) DeleteDownload(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_subtmgr_downloads WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete download: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// GetStats returns aggregated statistics for an account.
func (d *DB) GetStats(sourceAccountID string) (*SubtitleStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats SubtitleStats

	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_subtmgr_subtitles WHERE source_account_id = $1`,
		sourceAccountID).Scan(&stats.TotalSubtitles)
	if err != nil {
		return nil, fmt.Errorf("count subtitles: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_subtmgr_downloads WHERE source_account_id = $1`,
		sourceAccountID).Scan(&stats.TotalDownloads)
	if err != nil {
		return nil, fmt.Errorf("count downloads: %w", err)
	}

	langRows, err := d.pool.Query(ctx,
		`SELECT language, COUNT(*)::int AS count
		 FROM np_subtmgr_downloads
		 WHERE source_account_id = $1
		 GROUP BY language
		 ORDER BY count DESC`,
		sourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("language stats: %w", err)
	}
	defer langRows.Close()

	for langRows.Next() {
		var lc LanguageCount
		if err := langRows.Scan(&lc.Language, &lc.Count); err != nil {
			return nil, fmt.Errorf("scan language: %w", err)
		}
		stats.Languages = append(stats.Languages, lc)
	}
	if err := langRows.Err(); err != nil {
		return nil, err
	}

	srcRows, err := d.pool.Query(ctx,
		`SELECT source, COUNT(*)::int AS count
		 FROM np_subtmgr_downloads
		 WHERE source_account_id = $1
		 GROUP BY source
		 ORDER BY count DESC`,
		sourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("source stats: %w", err)
	}
	defer srcRows.Close()

	for srcRows.Next() {
		var sc SourceCount
		if err := srcRows.Scan(&sc.Source, &sc.Count); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		stats.Sources = append(stats.Sources, sc)
	}
	if err := srcRows.Err(); err != nil {
		return nil, err
	}

	return &stats, nil
}

// ---------------------------------------------------------------------------
// QC Results CRUD
// ---------------------------------------------------------------------------

// InsertQCResult inserts a new QC result record.
func (d *DB) InsertQCResult(data InsertQCResultInput) (*QCResultRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	accountID := data.SourceAccountID
	if accountID == "" {
		accountID = "primary"
	}

	checksJSON, err := json.Marshal(data.Checks)
	if err != nil {
		return nil, fmt.Errorf("marshal checks: %w", err)
	}
	issuesJSON, err := json.Marshal(data.Issues)
	if err != nil {
		return nil, fmt.Errorf("marshal issues: %w", err)
	}

	var qc QCResultRecord
	var checksRaw, issuesRaw []byte
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_subtmgr_qc_results
		    (source_account_id, download_id, status, checks, issues, cue_count, total_duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, source_account_id, download_id, status, checks, issues,
		           cue_count, total_duration_ms, created_at`,
		accountID, data.DownloadID, data.Status, checksJSON, issuesJSON,
		data.CueCount, data.TotalDurationMs,
	).Scan(&qc.ID, &qc.SourceAccountID, &qc.DownloadID, &qc.Status,
		&checksRaw, &issuesRaw, &qc.CueCount, &qc.TotalDurationMs, &qc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert qc result: %w", err)
	}

	if err := json.Unmarshal(checksRaw, &qc.Checks); err != nil {
		return nil, fmt.Errorf("unmarshal checks: %w", err)
	}
	if err := json.Unmarshal(issuesRaw, &qc.Issues); err != nil {
		return nil, fmt.Errorf("unmarshal issues: %w", err)
	}

	return &qc, nil
}

// UpdateDownloadQC updates the qc_status and qc_details on a download record.
func (d *DB) UpdateDownloadQC(downloadID, qcStatus string, details QualityCheckDetails) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal qc details: %w", err)
	}

	_, err = d.pool.Exec(ctx,
		`UPDATE np_subtmgr_downloads
		 SET qc_status = $1, qc_details = $2, updated_at = NOW()
		 WHERE id = $3`,
		qcStatus, detailsJSON, downloadID)
	if err != nil {
		return fmt.Errorf("update download qc: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nilIfEmpty(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nilIfEmptyStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZeroInt64(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

func nilIfZeroInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
