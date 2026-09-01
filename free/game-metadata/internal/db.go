package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the minimal subset of *pgxpool.Pool used by this plugin's
// handlers. Extracting it as an interface lets tests substitute a mock
// (e.g. pgxmock) without a live database, while production code continues
// to pass a real *pgxpool.Pool unchanged.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
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
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_game_platforms (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(255) NOT NULL,
			abbreviation VARCHAR(20),
			slug VARCHAR(255) NOT NULL,
			igdb_id INTEGER,
			generation INTEGER,
			manufacturer VARCHAR(255),
			platform_family VARCHAR(255),
			category VARCHAR(50),
			release_date DATE,
			summary TEXT,
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, slug)
		);

		CREATE INDEX IF NOT EXISTS idx_np_game_platforms_source_app ON np_game_platforms(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_platforms_slug ON np_game_platforms(source_account_id, slug);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_platforms_igdb ON np_game_platforms(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS np_game_genres (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name VARCHAR(255) NOT NULL,
			slug VARCHAR(255) NOT NULL,
			igdb_id INTEGER,
			description TEXT,
			parent_id UUID REFERENCES np_game_genres(id) ON DELETE SET NULL,
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, slug)
		);

		CREATE INDEX IF NOT EXISTS idx_np_game_genres_source_app ON np_game_genres(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_genres_slug ON np_game_genres(source_account_id, slug);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_genres_igdb ON np_game_genres(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS np_game_catalog (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			title VARCHAR(500) NOT NULL,
			slug VARCHAR(500) NOT NULL,
			platform_id UUID REFERENCES np_game_platforms(id) ON DELETE SET NULL,
			genre_id UUID REFERENCES np_game_genres(id) ON DELETE SET NULL,
			release_date DATE,
			developer VARCHAR(255),
			publisher VARCHAR(255),
			description TEXT,
			igdb_id INTEGER,
			rom_hash_md5 VARCHAR(32),
			rom_hash_sha1 VARCHAR(40),
			rom_hash_sha256 VARCHAR(64),
			rom_hash_crc32 VARCHAR(8),
			rom_filename VARCHAR(500),
			rom_size_bytes BIGINT,
			tier VARCHAR(50),
			rating DOUBLE PRECISION,
			players_min INTEGER DEFAULT 1,
			players_max INTEGER DEFAULT 1,
			is_verified BOOLEAN DEFAULT false,
			search_vector tsvector,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, slug, platform_id)
		);

		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_source_app ON np_game_catalog(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_title ON np_game_catalog(source_account_id, title);
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_platform ON np_game_catalog(source_account_id, platform_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_genre ON np_game_catalog(source_account_id, genre_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_tier ON np_game_catalog(source_account_id, tier);
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_search ON np_game_catalog USING GIN(search_vector);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_catalog_igdb ON np_game_catalog(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_md5 ON np_game_catalog(rom_hash_md5) WHERE rom_hash_md5 IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_sha1 ON np_game_catalog(rom_hash_sha1) WHERE rom_hash_sha1 IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_crc32 ON np_game_catalog(rom_hash_crc32) WHERE rom_hash_crc32 IS NOT NULL;

		CREATE TABLE IF NOT EXISTS np_game_metadata (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			game_id UUID NOT NULL REFERENCES np_game_catalog(id) ON DELETE CASCADE,
			source VARCHAR(50) NOT NULL DEFAULT 'igdb',
			igdb_id INTEGER,
			igdb_url TEXT,
			summary TEXT,
			storyline TEXT,
			total_rating DOUBLE PRECISION,
			total_rating_count INTEGER,
			aggregated_rating DOUBLE PRECISION,
			aggregated_rating_count INTEGER,
			first_release_date DATE,
			genres TEXT[] DEFAULT '{}',
			themes TEXT[] DEFAULT '{}',
			keywords TEXT[] DEFAULT '{}',
			game_modes TEXT[] DEFAULT '{}',
			franchises TEXT[] DEFAULT '{}',
			alternative_names TEXT[] DEFAULT '{}',
			websites JSONB DEFAULT '{}',
			age_ratings JSONB DEFAULT '{}',
			involved_companies JSONB DEFAULT '[]',
			raw_data JSONB DEFAULT '{}',
			fetched_at TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, game_id, source)
		);

		CREATE INDEX IF NOT EXISTS idx_np_game_metadata_source_app ON np_game_metadata(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_metadata_game ON np_game_metadata(game_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_metadata_igdb ON np_game_metadata(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS np_game_artwork (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			game_id UUID NOT NULL REFERENCES np_game_catalog(id) ON DELETE CASCADE,
			artwork_type VARCHAR(50) NOT NULL,
			url TEXT,
			local_path TEXT,
			width INTEGER,
			height INTEGER,
			mime_type VARCHAR(50),
			file_size_bytes BIGINT,
			source VARCHAR(50) NOT NULL DEFAULT 'igdb',
			igdb_image_id VARCHAR(50),
			is_primary BOOLEAN DEFAULT false,
			sort_order INTEGER DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_game_artwork_source_app ON np_game_artwork(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_artwork_game ON np_game_artwork(game_id);
		CREATE INDEX IF NOT EXISTS idx_np_game_artwork_type ON np_game_artwork(game_id, artwork_type);
	`

	_, err := pool.Exec(ctx, schema)
	return err
}
