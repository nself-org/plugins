package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Pipeline Runs
// =========================================================================

const pipelineColumns = `id, source_account_id, trigger_type, trigger_source,
  content_title, content_type, status,
  vpn_check_status, torrent_status, torrent_download_id,
  metadata_status, subtitle_status, encoding_status, encoding_job_id,
  publishing_status,
  detected_at, vpn_checked_at, torrent_submitted_at,
  download_completed_at, metadata_enriched_at, subtitles_fetched_at,
  encoding_completed_at, published_at, pipeline_completed_at,
  error_message, metadata, created_at, updated_at`

func scanPipelineRun(row pgx.Row) (*PipelineRun, error) {
	var p PipelineRun
	err := row.Scan(
		&p.ID, &p.SourceAccountID, &p.TriggerType, &p.TriggerSource,
		&p.ContentTitle, &p.ContentType, &p.Status,
		&p.VPNCheckStatus, &p.TorrentStatus, &p.TorrentDownloadID,
		&p.MetadataStatus, &p.SubtitleStatus, &p.EncodingStatus, &p.EncodingJobID,
		&p.PublishingStatus,
		&p.DetectedAt, &p.VPNCheckedAt, &p.TorrentSubmittedAt,
		&p.DownloadCompletedAt, &p.MetadataEnrichedAt, &p.SubtitlesFetchedAt,
		&p.EncodingCompletedAt, &p.PublishedAt, &p.PipelineCompletedAt,
		&p.ErrorMessage, &p.Metadata, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePipelineRun inserts a new pipeline run.
func (d *DB) CreatePipelineRun(accountID, triggerType string, triggerSource *string, contentTitle string, contentType *string, metadata json.RawMessage) (*PipelineRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_pipeline_runs
			   (source_account_id, trigger_type, trigger_source,
			    content_title, content_type, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING %s`, pipelineColumns),
		accountID, triggerType, triggerSource, contentTitle, contentType, metadata,
	)
	return scanPipelineRun(row)
}

// GetPipelineRun returns a single pipeline run by ID.
func (d *DB) GetPipelineRun(id int) (*PipelineRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_pipeline_runs WHERE id = $1`, pipelineColumns), id)
	p, err := scanPipelineRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListPipelineRuns returns paginated pipeline runs with optional status filter.
// Size-cap exception: single DB operation — 57L scan loop with struct mapping; splitting would fragment a single SQL query across files.
func (d *DB) ListPipelineRuns(status *string, limit, offset int) ([]PipelineRun, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	// Count
	countQuery := `SELECT COUNT(*)::int FROM np_contentacquisition_pipeline_runs`
	countArgs := []interface{}{}
	if status != nil {
		countQuery += ` WHERE status = $1`
		countArgs = append(countArgs, *status)
	}

	var total int
	if err := d.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data
	dataQuery := fmt.Sprintf(`SELECT %s FROM np_contentacquisition_pipeline_runs`, pipelineColumns)
	dataArgs := []interface{}{}
	idx := 1

	if status != nil {
		dataQuery += fmt.Sprintf(` WHERE status = $%d`, idx)
		dataArgs = append(dataArgs, *status)
		idx++
	}

	dataQuery += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	dataArgs = append(dataArgs, limit, offset)

	rows, err := d.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []PipelineRun
	for rows.Next() {
		var p PipelineRun
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.TriggerType, &p.TriggerSource,
			&p.ContentTitle, &p.ContentType, &p.Status,
			&p.VPNCheckStatus, &p.TorrentStatus, &p.TorrentDownloadID,
			&p.MetadataStatus, &p.SubtitleStatus, &p.EncodingStatus, &p.EncodingJobID,
			&p.PublishingStatus,
			&p.DetectedAt, &p.VPNCheckedAt, &p.TorrentSubmittedAt,
			&p.DownloadCompletedAt, &p.MetadataEnrichedAt, &p.SubtitlesFetchedAt,
			&p.EncodingCompletedAt, &p.PublishedAt, &p.PipelineCompletedAt,
			&p.ErrorMessage, &p.Metadata, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		runs = append(runs, p)
	}
	return runs, total, rows.Err()
}

