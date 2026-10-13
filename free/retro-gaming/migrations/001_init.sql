-- plugin-retro-gaming (game-metadata): Retro gaming ROM library schema.
-- Convention Wall: source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'.
-- Moved verbatim from internal/db.go InitSchema (the schema every existing install has), so a fresh install and an
-- upgrade end with the same tables.
-- Boot-applied by sdk/go/migrate with search_path pinned to np_retro_gaming, so every table is schema-qualified:
-- the tables live in public (the cli tracking rule) where the Go code and any older install already have them.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS public.np_retrogame_roms (
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
	ON public.np_retrogame_roms(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_platform
	ON public.np_retrogame_roms(source_account_id, platform);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_genre
	ON public.np_retrogame_roms(source_account_id, genre);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_title
	ON public.np_retrogame_roms(source_account_id, game_title_normalized);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_favorite
	ON public.np_retrogame_roms(source_account_id, favorite) WHERE favorite = true;
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_last_played
	ON public.np_retrogame_roms(source_account_id, last_played_at DESC NULLS LAST);

CREATE TABLE IF NOT EXISTS public.np_retrogame_save_states (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id VARCHAR(255) NOT NULL,
	rom_id UUID NOT NULL REFERENCES public.np_retrogame_roms(id) ON DELETE CASCADE,
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
	ON public.np_retrogame_save_states(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_rom
	ON public.np_retrogame_save_states(rom_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_user
	ON public.np_retrogame_save_states(source_account_id, user_id);

CREATE TABLE IF NOT EXISTS public.np_retrogame_play_sessions (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id VARCHAR(255) NOT NULL,
	rom_id UUID NOT NULL REFERENCES public.np_retrogame_roms(id) ON DELETE CASCADE,
	source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
	platform VARCHAR(50) NOT NULL,
	device_id VARCHAR(255),
	emulator_core VARCHAR(100) NOT NULL,
	started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	ended_at TIMESTAMPTZ,
	duration_seconds INTEGER,
	save_state_id UUID REFERENCES public.np_retrogame_save_states(id) ON DELETE SET NULL,
	auto_save_created BOOLEAN DEFAULT false,
	controller_type VARCHAR(50),
	created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_source
	ON public.np_retrogame_play_sessions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_rom
	ON public.np_retrogame_play_sessions(rom_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_user
	ON public.np_retrogame_play_sessions(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_sessions_started
	ON public.np_retrogame_play_sessions(source_account_id, started_at DESC);

CREATE TABLE IF NOT EXISTS public.np_retrogame_emulator_cores (
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
	ON public.np_retrogame_emulator_cores(platform);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_cores_recommended
	ON public.np_retrogame_emulator_cores(platform, is_recommended, priority);

CREATE TABLE IF NOT EXISTS public.np_retrogame_controller_configs (
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
	ON public.np_retrogame_controller_configs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_controllers_user
	ON public.np_retrogame_controller_configs(source_account_id, user_id);

CREATE TABLE IF NOT EXISTS public.np_retrogame_core_installations (
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
	ON public.np_retrogame_core_installations(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_installations_user
	ON public.np_retrogame_core_installations(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_installations_device
	ON public.np_retrogame_core_installations(source_account_id, device_id);
