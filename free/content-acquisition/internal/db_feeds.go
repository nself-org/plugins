package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// RSS Feeds
// =========================================================================

func scanRSSFeed(row pgx.Row) (*RSSFeed, error) {
	var f RSSFeed
	err := row.Scan(
		&f.ID, &f.SourceAccountID, &f.Name, &f.URL, &f.FeedType,
		&f.Enabled, &f.CheckIntervalMinutes, &f.QualityProfileID,
		&f.LastCheckAt, &f.LastSuccessAt, &f.LastError, &f.ConsecutiveFailures,
		&f.NextCheckAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

const rssFeedColumns = `id, source_account_id, name, url, feed_type,
  enabled, check_interval_minutes, quality_profile_id,
  last_check_at, last_success_at, last_error, consecutive_failures,
  next_check_at, created_at, updated_at`

// CreateRSSFeed inserts a new RSS feed.
func (d *DB) CreateRSSFeed(accountID, name, feedURL, feedType string) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_rss_feeds
			   (source_account_id, name, url, feed_type, check_interval_minutes)
			 VALUES ($1, $2, $3, $4, 60)
			 RETURNING %s`, rssFeedColumns),
		accountID, name, feedURL, feedType,
	)
	return scanRSSFeed(row)
}

// ListRSSFeeds returns all feeds for an account.
func (d *DB) ListRSSFeeds(accountID string) ([]RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT %s FROM np_contentacquisition_rss_feeds
			 WHERE source_account_id = $1
			 ORDER BY created_at DESC`, rssFeedColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []RSSFeed
	for rows.Next() {
		var f RSSFeed
		if err := rows.Scan(
			&f.ID, &f.SourceAccountID, &f.Name, &f.URL, &f.FeedType,
			&f.Enabled, &f.CheckIntervalMinutes, &f.QualityProfileID,
			&f.LastCheckAt, &f.LastSuccessAt, &f.LastError, &f.ConsecutiveFailures,
			&f.NextCheckAt, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

// UpdateRSSFeed updates allowed fields on a feed.
func (d *DB) UpdateRSSFeed(id string, req UpdateFeedRequest) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.URL != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", idx))
		args = append(args, *req.URL)
		idx++
	}
	if req.FeedType != nil {
		setClauses = append(setClauses, fmt.Sprintf("feed_type = $%d", idx))
		args = append(args, *req.FeedType)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}
	if req.CheckIntervalMinutes != nil {
		setClauses = append(setClauses, fmt.Sprintf("check_interval_minutes = $%d", idx))
		args = append(args, *req.CheckIntervalMinutes)
		idx++
	}

	if len(args) == 0 {
		return d.GetRSSFeed(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_rss_feeds SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, rssFeedColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	f, err := scanRSSFeed(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return f, err
}

// GetRSSFeed returns a single feed by ID.
func (d *DB) GetRSSFeed(id string) (*RSSFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_rss_feeds WHERE id = $1`, rssFeedColumns), id)
	f, err := scanRSSFeed(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return f, err
}

// DeleteRSSFeed deletes a feed by ID.
func (d *DB) DeleteRSSFeed(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_rss_feeds WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

