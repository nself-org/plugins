package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// =========================================================================
// Movie Monitoring
// =========================================================================

const movieColumns = `id, source_account_id, user_id, movie_title, tmdb_id,
  release_date, digital_release_date, quality_profile,
  auto_download, auto_upgrade, status, downloaded_quality,
  created_at, updated_at`

func scanMovie(row pgx.Row) (*MovieMonitoring, error) {
	var m MovieMonitoring
	err := row.Scan(
		&m.ID, &m.SourceAccountID, &m.UserID, &m.MovieTitle, &m.TmdbID,
		&m.ReleaseDate, &m.DigitalReleaseDate, &m.QualityProfile,
		&m.AutoDownload, &m.AutoUpgrade, &m.Status, &m.DownloadedQuality,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateMovieMonitoring adds a movie to the monitoring list.
func (d *DB) CreateMovieMonitoring(accountID, title string, tmdbID *int, qualityProfile string, autoDownload, autoUpgrade bool) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(
			`INSERT INTO np_contentacquisition_movie_monitoring
			   (source_account_id, user_id, movie_title, tmdb_id,
			    quality_profile, auto_download, auto_upgrade, status)
			 VALUES ($1, $1, $2, $3, $4, $5, $6, 'scheduled')
			 RETURNING %s`, movieColumns),
		accountID, title, tmdbID, qualityProfile, autoDownload, autoUpgrade,
	)
	return scanMovie(row)
}

// ListMovieMonitoring returns all monitored movies for an account.
func (d *DB) ListMovieMonitoring(accountID string) ([]MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_movie_monitoring
		 WHERE source_account_id = $1 ORDER BY created_at DESC`, movieColumns),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []MovieMonitoring
	for rows.Next() {
		var m MovieMonitoring
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.UserID, &m.MovieTitle, &m.TmdbID,
			&m.ReleaseDate, &m.DigitalReleaseDate, &m.QualityProfile,
			&m.AutoDownload, &m.AutoUpgrade, &m.Status, &m.DownloadedQuality,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}
	return movies, rows.Err()
}

// UpdateMovieMonitoring updates allowed fields on a monitored movie.
func (d *DB) UpdateMovieMonitoring(id string, req UpdateMovieRequest) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("movie_title = $%d", idx))
		args = append(args, *req.Title)
		idx++
	}
	if req.TmdbID != nil {
		setClauses = append(setClauses, fmt.Sprintf("tmdb_id = $%d", idx))
		args = append(args, *req.TmdbID)
		idx++
	}
	if req.QualityProfile != nil {
		setClauses = append(setClauses, fmt.Sprintf("quality_profile = $%d", idx))
		args = append(args, *req.QualityProfile)
		idx++
	}
	if req.AutoDownload != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_download = $%d", idx))
		args = append(args, *req.AutoDownload)
		idx++
	}
	if req.AutoUpgrade != nil {
		setClauses = append(setClauses, fmt.Sprintf("auto_upgrade = $%d", idx))
		args = append(args, *req.AutoUpgrade)
		idx++
	}
	if req.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, *req.Status)
		idx++
	}

	if len(args) == 0 {
		return d.GetMovieMonitoring(id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE np_contentacquisition_movie_monitoring SET %s WHERE id = $%d
		 RETURNING %s`,
		strings.Join(setClauses, ", "), idx, movieColumns,
	)

	row := d.pool.QueryRow(ctx, query, args...)
	m, err := scanMovie(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// GetMovieMonitoring returns a single monitored movie by ID.
func (d *DB) GetMovieMonitoring(id string) (*MovieMonitoring, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	row := d.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM np_contentacquisition_movie_monitoring WHERE id = $1`, movieColumns), id)
	m, err := scanMovie(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// DeleteMovieMonitoring removes a movie from monitoring.
func (d *DB) DeleteMovieMonitoring(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_contentacquisition_movie_monitoring WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

