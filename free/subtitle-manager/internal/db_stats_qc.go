package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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
