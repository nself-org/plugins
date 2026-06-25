package internal

import (
	"context"
	"fmt"
	"time"
)

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
