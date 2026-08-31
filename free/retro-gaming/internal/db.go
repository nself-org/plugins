package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_retrogame_roms (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			rom_file_path TEXT NOT NULL,
			rom_file_size_bytes BIGINT,
			rom_file_hash VARCHAR(128),
			game_title VARCHAR(500) NOT NULL,
			game_title_normalized VARCHAR(500) NOT NULL,
			platform VARCHAR(50) NOT NULL,
			region VARCHAR(20),
			release_year INTEGER,
			genre VARCHAR(100),
			publisher VARCHAR(255),
			developer VARCHAR(255),
			igdb_id INTEGER,
			moby_games_id INTEGER,
			box_art_url TEXT,
			box_art_local_path TEXT,
			screenshot_urls TEXT[] DEFAULT '{}',
			screenshot_local_paths TEXT[] DEFAULT '{}',
			description TEXT,
			description_source VARCHAR(50),
			recommended_core VARCHAR(100),
			core_overrides JSONB DEFAULT '{}',
			user_rating DOUBLE PRECISION,
			play_count INTEGER DEFAULT 0,
			last_played_at TIMESTAMPTZ,
			favorite BOOLEAN DEFAULT false,
			scan_source VARCHAR(100),
			added_by_user_id VARCHAR(255),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, rom_file_hash),
			UNIQUE(source_account_id, rom_file_path)
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_source
			ON np_retrogame_roms(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_platform
			ON np_retrogame_roms(source_account_id, platform);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_genre
			ON np_retrogame_roms(source_account_id, genre);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_title
			ON np_retrogame_roms(source_account_id, game_title_normalized);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_favorite
			ON np_retrogame_roms(source_account_id, favorite) WHERE favorite = true;
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_last_played
			ON np_retrogame_roms(source_account_id, last_played_at DESC NULLS LAST);

		CREATE TABLE IF NOT EXISTS np_retrogame_save_states (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id VARCHAR(255) NOT NULL,
			rom_id UUID NOT NULL REFERENCES np_retrogame_roms(id) ON DELETE CASCADE,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			slot INTEGER NOT NULL,
			save_state_file_path TEXT NOT NULL,
			save_state_file_size_bytes BIGINT,
			screenshot_url TEXT,
			screenshot_local_path TEXT,
			emulator_core VARCHAR(100) NOT NULL,
			emulator_version VARCHAR(50),
			description TEXT,
			play_time_seconds INTEGER DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, user_id, rom_id, slot)
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_source
			ON np_retrogame_save_states(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_rom
			ON np_retrogame_save_states(rom_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_user
			ON np_retrogame_save_states(source_account_id, user_id);

		CREATE TABLE IF NOT EXISTS np_retrogame_play_sessions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id VARCHAR(255) NOT NULL,
			rom_id UUID NOT NULL REFERENCES np_retrogame_roms(id) ON DELETE CASCADE,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			platform VARCHAR(50) NOT NULL,
			device_id VARCHAR(255),
			emulator_core VARCHAR(100) NOT NULL,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ended_at TIMESTAMPTZ,
			duration_seconds INTEGER,
			save_state_id UUID REFERENCES np_retrogame_save_states(id) ON DELETE SET NULL,
			auto_save_created BOOLEAN DEFAULT false,
			controller_type VARCHAR(50),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_source
			ON np_retrogame_play_sessions(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_rom
			ON np_retrogame_play_sessions(rom_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_user
			ON np_retrogame_play_sessions(source_account_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_started
			ON np_retrogame_play_sessions(source_account_id, started_at DESC);

		CREATE TABLE IF NOT EXISTS np_retrogame_emulator_cores (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			core_name VARCHAR(100) NOT NULL,
			display_name VARCHAR(255) NOT NULL,
			platform VARCHAR(50) NOT NULL,
			core_wasm_path TEXT,
			core_wasm_size_bytes BIGINT,
			version VARCHAR(50) NOT NULL,
			license VARCHAR(100),
			author VARCHAR(255),
			homepage_url TEXT,
			supports_save_states BOOLEAN DEFAULT true,
			supports_rewind BOOLEAN DEFAULT false,
			supports_fast_forward BOOLEAN DEFAULT true,
			supports_cheats BOOLEAN DEFAULT false,
			default_config JSONB DEFAULT '{}',
			is_recommended BOOLEAN DEFAULT false,
			priority INTEGER DEFAULT 10,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(core_name, platform)
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_cores_platform
			ON np_retrogame_emulator_cores(platform);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_cores_recommended
			ON np_retrogame_emulator_cores(platform, is_recommended, priority);

		CREATE TABLE IF NOT EXISTS np_retrogame_controller_configs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			user_id VARCHAR(255) NOT NULL,
			config_name VARCHAR(255) NOT NULL,
			platform VARCHAR(50),
			controller_type VARCHAR(50) NOT NULL,
			button_mapping JSONB NOT NULL DEFAULT '{}',
			touch_layout JSONB DEFAULT '{}',
			analog_sensitivity DOUBLE PRECISION DEFAULT 1.0,
			vibration_enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, user_id, config_name)
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_controllers_source
			ON np_retrogame_controller_configs(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_controllers_user
			ON np_retrogame_controller_configs(source_account_id, user_id);

		CREATE TABLE IF NOT EXISTS np_retrogame_core_installations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id VARCHAR(255) NOT NULL,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			device_id VARCHAR(255) NOT NULL,
			device_platform VARCHAR(50) NOT NULL,
			core_name VARCHAR(100) NOT NULL,
			core_version VARCHAR(50) NOT NULL,
			installed_at TIMESTAMPTZ DEFAULT NOW(),
			last_used_at TIMESTAMPTZ,
			UNIQUE(source_account_id, user_id, device_id, core_name)
		);

		CREATE INDEX IF NOT EXISTS idx_np_retrogame_installations_source
			ON np_retrogame_core_installations(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_installations_user
			ON np_retrogame_core_installations(source_account_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_np_retrogame_installations_device
			ON np_retrogame_core_installations(source_account_id, device_id);
	`

	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}
