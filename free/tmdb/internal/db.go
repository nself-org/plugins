package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used throughout this
// package. It exists as a testability seam: production code passes a
// *pgxpool.Pool, tests pass a pgxmock.PgxPoolIface — both satisfy this
// interface structurally, so handlers/db helpers are unit-testable without a
// live Postgres instance.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

func NewDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	return pool, nil
}

func InitSchema(ctx context.Context, pool Querier) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS np_tmdb_movies (
			id INTEGER PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			imdb_id VARCHAR(20),
			title VARCHAR(500) NOT NULL,
			original_title VARCHAR(500),
			overview TEXT,
			tagline TEXT,
			release_date DATE,
			runtime INTEGER,
			status VARCHAR(50),
			poster_path VARCHAR(255),
			backdrop_path VARCHAR(255),
			budget BIGINT,
			revenue BIGINT,
			vote_average DOUBLE PRECISION,
			vote_count INTEGER,
			popularity DOUBLE PRECISION,
			original_language VARCHAR(10),
			genres JSONB DEFAULT '[]',
			production_companies JSONB DEFAULT '[]',
			production_countries JSONB DEFAULT '[]',
			spoken_languages JSONB DEFAULT '[]',
			credits JSONB DEFAULT '{}',
			keywords JSONB DEFAULT '[]',
			content_rating VARCHAR(20),
			synced_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_movies_source_app ON np_tmdb_movies(source_account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_movies_imdb ON np_tmdb_movies(imdb_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_movies_title ON np_tmdb_movies(title)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_tv_shows (
			id INTEGER PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			imdb_id VARCHAR(20),
			name VARCHAR(500) NOT NULL,
			original_name VARCHAR(500),
			overview TEXT,
			first_air_date DATE,
			last_air_date DATE,
			status VARCHAR(50),
			type VARCHAR(50),
			number_of_seasons INTEGER,
			number_of_episodes INTEGER,
			episode_run_time INTEGER[],
			poster_path VARCHAR(255),
			backdrop_path VARCHAR(255),
			vote_average DOUBLE PRECISION,
			vote_count INTEGER,
			popularity DOUBLE PRECISION,
			original_language VARCHAR(10),
			genres JSONB DEFAULT '[]',
			networks JSONB DEFAULT '[]',
			created_by JSONB DEFAULT '[]',
			credits JSONB DEFAULT '{}',
			content_rating VARCHAR(20),
			synced_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_tv_source_app ON np_tmdb_tv_shows(source_account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_tv_imdb ON np_tmdb_tv_shows(imdb_id)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_tv_seasons (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			show_id INTEGER NOT NULL REFERENCES np_tmdb_tv_shows(id),
			season_number INTEGER NOT NULL,
			name VARCHAR(500),
			overview TEXT,
			poster_path VARCHAR(255),
			air_date DATE,
			episode_count INTEGER,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(show_id, season_number)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_seasons_source_app ON np_tmdb_tv_seasons(source_account_id)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_tv_episodes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			show_id INTEGER NOT NULL REFERENCES np_tmdb_tv_shows(id),
			season_number INTEGER NOT NULL,
			episode_number INTEGER NOT NULL,
			name VARCHAR(500),
			overview TEXT,
			still_path VARCHAR(255),
			air_date DATE,
			runtime INTEGER,
			vote_average DOUBLE PRECISION,
			crew JSONB DEFAULT '[]',
			guest_stars JSONB DEFAULT '[]',
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(show_id, season_number, episode_number)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_episodes_source_app ON np_tmdb_tv_episodes(source_account_id)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_genres (
			id INTEGER PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(100) NOT NULL,
			media_type VARCHAR(10) NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_genres_source_app ON np_tmdb_genres(source_account_id)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_match_queue (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			media_id VARCHAR(255) NOT NULL,
			filename VARCHAR(500),
			parsed_title VARCHAR(500),
			parsed_year INTEGER,
			parsed_type VARCHAR(20),
			match_results JSONB DEFAULT '[]',
			best_match_id INTEGER,
			best_match_type VARCHAR(20),
			confidence DOUBLE PRECISION,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			reviewed_by VARCHAR(255),
			reviewed_at TIMESTAMPTZ,
			auto_accepted BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_match_source_app ON np_tmdb_match_queue(source_account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_match_status ON np_tmdb_match_queue(source_account_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_match_media ON np_tmdb_match_queue(source_account_id, media_id)`,

		`CREATE TABLE IF NOT EXISTS np_tmdb_webhook_events (
			id VARCHAR(255) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			event_type VARCHAR(128) NOT NULL,
			payload JSONB NOT NULL,
			processed BOOLEAN DEFAULT false,
			processed_at TIMESTAMPTZ,
			error TEXT,
			retry_count INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_tmdb_webhook_events_source ON np_tmdb_webhook_events(source_account_id)`,
	}

	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	return nil
}

// CountMovies returns total movies for a source account.
func CountMovies(ctx context.Context, pool Querier, sourceAccountID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_movies WHERE source_account_id = $1`,
		sourceAccountID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting movies: %w", err)
	}
	return n, nil
}

// CountTVShows returns total TV shows for a source account.
func CountTVShows(ctx context.Context, pool Querier, sourceAccountID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_tv_shows WHERE source_account_id = $1`,
		sourceAccountID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting tv shows: %w", err)
	}
	return n, nil
}

// CountPendingMatches returns number of match queue entries with status 'pending'.
func CountPendingMatches(ctx context.Context, pool Querier, sourceAccountID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_match_queue WHERE source_account_id = $1 AND status = 'pending'`,
		sourceAccountID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting pending matches: %w", err)
	}
	return n, nil
}

// CountGenres returns total genres for a source account.
func CountGenres(ctx context.Context, pool Querier, sourceAccountID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_genres WHERE source_account_id = $1`,
		sourceAccountID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting genres: %w", err)
	}
	return n, nil
}

// GetQueue returns match queue entries for a source account, optionally filtered by status.
func GetQueue(ctx context.Context, pool Querier, sourceAccountID, status string, limit, offset int) ([]MatchQueue, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, source_account_id, media_id, filename, parsed_title, parsed_year,
		parsed_type, match_results, best_match_id, best_match_type, confidence,
		status, reviewed_by, reviewed_at, auto_accepted, created_at, updated_at
		FROM np_tmdb_match_queue
		WHERE source_account_id = $1`
	args := []interface{}{sourceAccountID}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying match queue: %w", err)
	}
	defer rows.Close()

	var items []MatchQueue
	for rows.Next() {
		var m MatchQueue
		if err := rows.Scan(
			&m.ID, &m.SourceAccountID, &m.MediaID, &m.Filename, &m.ParsedTitle, &m.ParsedYear,
			&m.ParsedType, &m.MatchResults, &m.BestMatchID, &m.BestMatchType, &m.Confidence,
			&m.Status, &m.ReviewedBy, &m.ReviewedAt, &m.AutoAccepted, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning match queue row: %w", err)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// InsertMatchQueueEntry inserts a new match queue entry and returns the generated ID.
func InsertMatchQueueEntry(ctx context.Context, pool Querier, m *MatchQueue) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO np_tmdb_match_queue
		(source_account_id, media_id, filename, parsed_title, parsed_year, parsed_type,
		 match_results, best_match_id, best_match_type, confidence, status, auto_accepted)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`,
		m.SourceAccountID, m.MediaID, m.Filename, m.ParsedTitle, m.ParsedYear, m.ParsedType,
		m.MatchResults, m.BestMatchID, m.BestMatchType, m.Confidence, m.Status, m.AutoAccepted,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting match queue entry: %w", err)
	}
	return id, nil
}

// ConfirmMatchQueueEntry confirms a match, setting the best match and status to 'confirmed'.
func ConfirmMatchQueueEntry(ctx context.Context, pool Querier, queueID string, tmdbID int, mediaType, reviewedBy, sourceAccountID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE np_tmdb_match_queue
		SET best_match_id = $1, best_match_type = $2, status = 'confirmed',
		    reviewed_by = $3, reviewed_at = NOW(), updated_at = NOW()
		WHERE id = $4 AND source_account_id = $5`,
		tmdbID, mediaType, reviewedBy, queueID, sourceAccountID,
	)
	if err != nil {
		return fmt.Errorf("confirming match: %w", err)
	}
	return nil
}

// UpsertMovie inserts or updates a movie record.
func UpsertMovie(ctx context.Context, pool Querier, m *TMDBMovieDetails, sourceAccountID string) error {
	genres, _ := jsonMarshal(m.Genres)
	_, err := pool.Exec(ctx,
		`INSERT INTO np_tmdb_movies
		(id, source_account_id, imdb_id, title, original_title, overview, tagline,
		 release_date, runtime, status, poster_path, backdrop_path, budget, revenue,
		 vote_average, vote_count, popularity, original_language, genres,
		 production_companies, production_countries, spoken_languages, credits, keywords, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,
		        NULLIF($8,'')::DATE,$9,$10,$11,$12,$13,$14,
		        $15,$16,$17,$18,$19,$20,$21,$22,$23,$24,NOW())
		ON CONFLICT (id) DO UPDATE SET
		  source_account_id=$2, imdb_id=$3, title=$4, original_title=$5, overview=$6,
		  tagline=$7, release_date=NULLIF($8,'')::DATE, runtime=$9, status=$10,
		  poster_path=$11, backdrop_path=$12, budget=$13, revenue=$14,
		  vote_average=$15, vote_count=$16, popularity=$17, original_language=$18,
		  genres=$19, production_companies=$20, production_countries=$21,
		  spoken_languages=$22, credits=$23, keywords=$24, synced_at=NOW()`,
		m.ID, sourceAccountID, nullString(m.ImdbID), m.Title, nullString(m.OriginalTitle),
		nullString(m.Overview), nullString(m.Tagline),
		m.ReleaseDate, nullInt(m.Runtime), nullString(m.Status),
		nullString(m.PosterPath), nullString(m.BackdropPath),
		nullInt64(m.Budget), nullInt64(m.Revenue),
		m.VoteAverage, m.VoteCount, m.Popularity, m.OriginalLanguage,
		genres, m.ProductionCompanies, m.ProductionCountries, m.SpokenLanguages, m.Credits, m.Keywords,
	)
	if err != nil {
		return fmt.Errorf("upserting movie: %w", err)
	}
	return nil
}

// UpsertTVShow inserts or updates a TV show record.
func UpsertTVShow(ctx context.Context, pool Querier, t *TMDBTVDetails, sourceAccountID string) error {
	genres, _ := jsonMarshal(t.Genres)
	episodeRunTime, _ := jsonMarshal(t.EpisodeRunTime)
	_, err := pool.Exec(ctx,
		`INSERT INTO np_tmdb_tv_shows
		(id, source_account_id, imdb_id, name, original_name, overview,
		 first_air_date, last_air_date, status, type, number_of_seasons, number_of_episodes,
		 episode_run_time, poster_path, backdrop_path, vote_average, vote_count,
		 popularity, original_language, genres, networks, created_by, credits, synced_at)
		VALUES ($1,$2,$3,$4,$5,$6,
		        NULLIF($7,'')::DATE,NULLIF($8,'')::DATE,$9,$10,$11,$12,
		        $13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,NOW())
		ON CONFLICT (id) DO UPDATE SET
		  source_account_id=$2, imdb_id=$3, name=$4, original_name=$5, overview=$6,
		  first_air_date=NULLIF($7,'')::DATE, last_air_date=NULLIF($8,'')::DATE,
		  status=$9, type=$10, number_of_seasons=$11, number_of_episodes=$12,
		  episode_run_time=$13, poster_path=$14, backdrop_path=$15,
		  vote_average=$16, vote_count=$17, popularity=$18, original_language=$19,
		  genres=$20, networks=$21, created_by=$22, credits=$23, synced_at=NOW()`,
		t.ID, sourceAccountID, nullString(t.ExternalIDs.ImdbID), t.Name, nullString(t.OriginalName),
		nullString(t.Overview), t.FirstAirDate, t.LastAirDate, nullString(t.Status), nullString(t.Type),
		nullInt(t.NumberOfSeasons), nullInt(t.NumberOfEpisodes),
		episodeRunTime, nullString(t.PosterPath), nullString(t.BackdropPath),
		t.VoteAverage, t.VoteCount, t.Popularity, t.OriginalLanguage,
		genres, t.Networks, t.CreatedBy, t.Credits,
	)
	if err != nil {
		return fmt.Errorf("upserting tv show: %w", err)
	}
	return nil
}

// UpsertGenres inserts or updates genre records for a source account.
func UpsertGenres(ctx context.Context, pool Querier, genres []TMDBGenre, mediaType, sourceAccountID string) error {
	for _, g := range genres {
		_, err := pool.Exec(ctx,
			`INSERT INTO np_tmdb_genres (id, source_account_id, name, media_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET name=$3, media_type=$4`,
			g.ID, sourceAccountID, g.Name, mediaType,
		)
		if err != nil {
			return fmt.Errorf("upserting genre %d: %w", g.ID, err)
		}
	}
	return nil
}

// helpers

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

func nullInt64(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

func jsonMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("[]"), nil
	}
	if raw, ok := v.([]byte); ok {
		return raw, nil
	}
	return json.Marshal(v)
}
