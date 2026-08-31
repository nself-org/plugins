package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_sports_leagues (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			external_id VARCHAR(255) NOT NULL,
			provider VARCHAR(64) NOT NULL,
			name VARCHAR(255) NOT NULL,
			abbreviation VARCHAR(32),
			sport VARCHAR(64) NOT NULL,
			country VARCHAR(3),
			season_type VARCHAR(32),
			current_season VARCHAR(32),
			logo_url TEXT,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, provider, external_id)
		);
		CREATE INDEX IF NOT EXISTS idx_np_sports_leagues_source_account ON np_sports_leagues(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_leagues_sport ON np_sports_leagues(sport);
		CREATE INDEX IF NOT EXISTS idx_np_sports_leagues_provider ON np_sports_leagues(provider);

		CREATE TABLE IF NOT EXISTS np_sports_teams (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			league_id UUID REFERENCES np_sports_leagues(id),
			external_id VARCHAR(255) NOT NULL,
			provider VARCHAR(64) NOT NULL,
			name VARCHAR(255) NOT NULL,
			abbreviation VARCHAR(16),
			city VARCHAR(128),
			conference VARCHAR(128),
			division VARCHAR(128),
			logo_url TEXT,
			primary_color VARCHAR(7),
			secondary_color VARCHAR(7),
			venue_name VARCHAR(255),
			venue_city VARCHAR(128),
			venue_timezone VARCHAR(64),
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, provider, external_id)
		);
		CREATE INDEX IF NOT EXISTS idx_np_sports_teams_source_account ON np_sports_teams(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_teams_league ON np_sports_teams(league_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_teams_abbr ON np_sports_teams(abbreviation);

		CREATE TABLE IF NOT EXISTS np_sports_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			external_id VARCHAR(255) NOT NULL,
			provider VARCHAR(64) NOT NULL,
			canonical_id VARCHAR(255),
			league_id UUID REFERENCES np_sports_leagues(id),
			home_team_id UUID REFERENCES np_sports_teams(id),
			away_team_id UUID REFERENCES np_sports_teams(id),
			event_type VARCHAR(32) NOT NULL DEFAULT 'regular',
			status VARCHAR(32) NOT NULL DEFAULT 'scheduled',
			scheduled_at TIMESTAMPTZ NOT NULL,
			started_at TIMESTAMPTZ,
			ended_at TIMESTAMPTZ,
			venue_name VARCHAR(255),
			venue_city VARCHAR(128),
			venue_timezone VARCHAR(64),
			broadcast_network VARCHAR(128),
			broadcast_channel VARCHAR(128),
			season VARCHAR(32),
			season_type VARCHAR(32),
			week INTEGER,
			home_score INTEGER,
			away_score INTEGER,
			period VARCHAR(32),
			clock VARCHAR(16),
			is_final BOOLEAN DEFAULT FALSE,
			is_locked BOOLEAN DEFAULT FALSE,
			lock_reason VARCHAR(255),
			locked_at TIMESTAMPTZ,
			operator_override BOOLEAN DEFAULT FALSE,
			operator_notes TEXT,
			recording_trigger_sent BOOLEAN DEFAULT FALSE,
			recording_trigger_sent_at TIMESTAMPTZ,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			UNIQUE(source_account_id, provider, external_id)
		);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_source_account ON np_sports_events(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_league ON np_sports_events(league_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_scheduled ON np_sports_events(scheduled_at);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_status ON np_sports_events(status);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_home ON np_sports_events(home_team_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_away ON np_sports_events(away_team_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_canonical ON np_sports_events(canonical_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_events_season ON np_sports_events(season, week);

		CREATE TABLE IF NOT EXISTS sports_provider_syncs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			provider VARCHAR(64) NOT NULL,
			resource_type VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			records_synced INTEGER DEFAULT 0,
			errors JSONB DEFAULT '[]',
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_sports_syncs_source_account ON sports_provider_syncs(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_sports_syncs_provider ON sports_provider_syncs(provider);

		CREATE TABLE IF NOT EXISTS sports_schedule_cache (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			provider VARCHAR(64) NOT NULL,
			cache_key VARCHAR(255) NOT NULL,
			data JSONB NOT NULL,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL,
			UNIQUE(source_account_id, provider, cache_key)
		);
		CREATE INDEX IF NOT EXISTS idx_sports_cache_source_account ON sports_schedule_cache(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_sports_cache_expires ON sports_schedule_cache(expires_at);

		CREATE TABLE IF NOT EXISTS sports_webhook_events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			provider VARCHAR(64) NOT NULL,
			event_type VARCHAR(128) NOT NULL,
			event_id VARCHAR(255),
			payload JSONB NOT NULL,
			processed BOOLEAN DEFAULT FALSE,
			processed_at TIMESTAMPTZ,
			error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_sports_webhooks_source_account ON sports_webhook_events(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_sports_webhooks_type ON sports_webhook_events(event_type);
		CREATE INDEX IF NOT EXISTS idx_sports_webhooks_processed ON sports_webhook_events(processed);

		CREATE TABLE IF NOT EXISTS np_sports_standings (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			league_id UUID NOT NULL REFERENCES np_sports_leagues(id),
			team_id UUID NOT NULL REFERENCES np_sports_teams(id),
			wins INTEGER NOT NULL DEFAULT 0,
			losses INTEGER NOT NULL DEFAULT 0,
			draws INTEGER NOT NULL DEFAULT 0,
			points INTEGER NOT NULL DEFAULT 0,
			rank INTEGER NOT NULL DEFAULT 0,
			season VARCHAR(32) NOT NULL,
			synced_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, league_id, team_id, season)
		);
		CREATE INDEX IF NOT EXISTS idx_np_sports_standings_source_account ON np_sports_standings(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_standings_league ON np_sports_standings(league_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_standings_team ON np_sports_standings(team_id);
		CREATE INDEX IF NOT EXISTS idx_np_sports_standings_season ON np_sports_standings(season);

		CREATE TABLE IF NOT EXISTS np_np_sports_favorites (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			user_id VARCHAR(255) NOT NULL,
			team_id UUID NOT NULL REFERENCES np_sports_teams(id),
			notify_live BOOLEAN DEFAULT TRUE,
			auto_record BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, user_id, team_id)
		);
		CREATE INDEX IF NOT EXISTS idx_np_np_sports_favorites_source_account ON np_np_sports_favorites(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_np_sports_favorites_user ON np_np_sports_favorites(user_id);
		CREATE INDEX IF NOT EXISTS idx_np_np_sports_favorites_team ON np_np_sports_favorites(team_id);
	`
	_, err := pool.Exec(ctx, schema)
	return err
}
