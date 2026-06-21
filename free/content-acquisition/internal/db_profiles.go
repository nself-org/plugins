package internal

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Quality Profiles
// =========================================================================

// CreateQualityProfile inserts a new quality profile.
func (d *DB) CreateQualityProfile(accountID, name string, preferredQualities []string, minSeeders int) (*QualityProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if preferredQualities == nil {
		preferredQualities = []string{"1080p", "720p"}
	}

	row := d.pool.QueryRow(ctx,
		`INSERT INTO np_contentacquisition_quality_profiles
		   (source_account_id, name, preferred_qualities, min_seeders)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, source_account_id, name, description, preferred_qualities,
		   max_size_gb, min_size_gb, preferred_sources, excluded_sources,
		   preferred_groups, excluded_groups, preferred_languages,
		   require_subtitles, min_seeders, wait_for_better_quality, wait_hours,
		   created_at, updated_at`,
		accountID, name, preferredQualities, minSeeders,
	)
	return scanQualityProfile(row)
}

func scanQualityProfile(row pgx.Row) (*QualityProfile, error) {
	var p QualityProfile
	err := row.Scan(
		&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
		&p.PreferredQualities, &p.MaxSizeGB, &p.MinSizeGB,
		&p.PreferredSources, &p.ExcludedSources,
		&p.PreferredGroups, &p.ExcludedGroups, &p.PreferredLanguages,
		&p.RequireSubtitles, &p.MinSeeders, &p.WaitForBetter, &p.WaitHours,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProfiles lists all quality profiles for an account.
func (d *DB) ListProfiles(accountID string) ([]QualityProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, name, description, preferred_qualities,
		   max_size_gb, min_size_gb, preferred_sources, excluded_sources,
		   preferred_groups, excluded_groups, preferred_languages,
		   require_subtitles, min_seeders, wait_for_better_quality, wait_hours,
		   created_at, updated_at
		 FROM np_contentacquisition_quality_profiles
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []QualityProfile
	for rows.Next() {
		var p QualityProfile
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
			&p.PreferredQualities, &p.MaxSizeGB, &p.MinSizeGB,
			&p.PreferredSources, &p.ExcludedSources,
			&p.PreferredGroups, &p.ExcludedGroups, &p.PreferredLanguages,
			&p.RequireSubtitles, &p.MinSeeders, &p.WaitForBetter, &p.WaitHours,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

