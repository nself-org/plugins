package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"fmt"
	"time"
)

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
