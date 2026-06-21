package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Subscriptions
// =========================================================================

func scanSubscription(row pgx.Row) (*Subscription, error) {
	var s Subscription
	err := row.Scan(
		&s.ID, &s.SourceAccountID, &s.SubscriptionType, &s.ContentID,
		&s.ContentName, &s.ContentMetadata, &s.QualityProfileID,
		&s.Enabled, &s.AutoUpgrade, &s.MonitorFutureSeasons,
		&s.MonitorExistingSeasons, &s.SeasonFolder,
		&s.LastCheckAt, &s.LastDownloadAt, &s.NextCheckAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateSubscription inserts a new subscription.
func (d *DB) CreateSubscription(accountID, subType string, contentID *string, contentName string, qualityProfileID *string) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		`INSERT INTO np_contentacquisition_acquisition_subscriptions
		   (source_account_id, subscription_type, content_id, content_name,
		    content_metadata, quality_profile_id, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, true)
		 RETURNING id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at`,
		accountID, subType, contentID, contentName, json.RawMessage("{}"), qualityProfileID,
	)
	return scanSubscription(row)
}

// GetSubscription returns a single subscription by ID.
func (d *DB) GetSubscription(id string) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at
		 FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`, id)
	s, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSubscriptions returns all subscriptions for an account.
func (d *DB) ListSubscriptions(accountID string) ([]Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at
		 FROM np_contentacquisition_acquisition_subscriptions
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(
			&s.ID, &s.SourceAccountID, &s.SubscriptionType, &s.ContentID,
			&s.ContentName, &s.ContentMetadata, &s.QualityProfileID,
			&s.Enabled, &s.AutoUpgrade, &s.MonitorFutureSeasons,
			&s.MonitorExistingSeasons, &s.SeasonFolder,
			&s.LastCheckAt, &s.LastDownloadAt, &s.NextCheckAt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// UpdateSubscription updates allowed fields on a subscription.
func (d *DB) UpdateSubscription(id string, req UpdateSubscriptionRequest) (*Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.ContentType != nil {
		setClauses = append(setClauses, fmt.Sprintf("subscription_type = $%d", idx))
		args = append(args, *req.ContentType)
		idx++
	}
	if req.ContentID != nil {
		setClauses = append(setClauses, fmt.Sprintf("content_id = $%d", idx))
		args = append(args, *req.ContentID)
		idx++
	}
	if req.ContentName != nil {
		setClauses = append(setClauses, fmt.Sprintf("content_name = $%d", idx))
		args = append(args, *req.ContentName)
		idx++
	}
	if req.QualityProfileID != nil {
		setClauses = append(setClauses, fmt.Sprintf("quality_profile_id = $%d", idx))
		args = append(args, *req.QualityProfileID)
		idx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *req.Enabled)
		idx++
	}
	if req.AutoUpgrade != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_upgrade = $%d", idx))
		args = append(args, *req.AutoUpgrade)
		idx++
	}

	if len(args) == 0 {
		return d.GetSubscription(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_acquisition_subscriptions SET %s WHERE id = $%d
		 RETURNING id, source_account_id, subscription_type, content_id,
		   content_name, content_metadata, quality_profile_id,
		   enabled, auto_upgrade, monitor_future_seasons,
		   monitor_existing_seasons, season_folder,
		   last_check_at, last_download_at, next_check_at,
		   created_at, updated_at`,
		strings.Join(setClauses, ", "), idx,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	s, err := scanSubscription(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// DeleteSubscription deletes a subscription. Returns true if a row was deleted.
func (d *DB) DeleteSubscription(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_acquisition_subscriptions WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

