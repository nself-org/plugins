-- plugin-retro-gaming (game-metadata): Retro gaming ROM library schema
-- Convention Wall: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Columns from handlers.go INSERT statements (code wins)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_retrogame_roms (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id       TEXT        NOT NULL DEFAULT 'primary',
    rom_file_path           TEXT        NOT NULL,
    rom_file_size_bytes     BIGINT,
    rom_file_hash           TEXT,
    game_title              TEXT        NOT NULL,
    game_title_normalized   TEXT,
    platform                TEXT        NOT NULL,
    region                  TEXT,
    release_year            INTEGER,
    genre                   TEXT,
    publisher               TEXT,
    developer               TEXT,
    igdb_id                 TEXT,
    moby_games_id           TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_source_account ON np_retrogame_roms(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_roms_platform ON np_retrogame_roms(platform);

CREATE TABLE IF NOT EXISTS np_retrogame_save_states (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id       TEXT        NOT NULL DEFAULT 'primary',
    user_id                 TEXT        NOT NULL,
    rom_id                  UUID        REFERENCES np_retrogame_roms(id) ON DELETE CASCADE,
    slot                    INTEGER     NOT NULL DEFAULT 0,
    save_state_file_path    TEXT,
    save_state_file_size_bytes BIGINT,
    screenshot_url          TEXT,
    screenshot_local_path   TEXT,
    emulator_core           TEXT,
    emulator_version        TEXT,
    description             TEXT,
    play_time_seconds       INTEGER     NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_save_states_source_account ON np_retrogame_save_states(source_account_id);

CREATE TABLE IF NOT EXISTS np_retrogame_play_sessions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    rom_id            UUID        REFERENCES np_retrogame_roms(id) ON DELETE CASCADE,
    platform          TEXT,
    device_id         TEXT,
    emulator_core     TEXT,
    controller_type   TEXT,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at          TIMESTAMPTZ,
    duration_seconds  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_play_sessions_source_account ON np_retrogame_play_sessions(source_account_id);

CREATE TABLE IF NOT EXISTS np_retrogame_emulator_cores (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    core_name           TEXT        NOT NULL UNIQUE,
    display_name        TEXT        NOT NULL,
    platform            TEXT        NOT NULL,
    core_wasm_path      TEXT,
    version             TEXT,
    license             TEXT,
    author              TEXT,
    homepage_url        TEXT,
    supports_save_states BOOLEAN   NOT NULL DEFAULT TRUE,
    supports_rewind     BOOLEAN     NOT NULL DEFAULT FALSE,
    supports_fast_forward BOOLEAN  NOT NULL DEFAULT FALSE,
    supports_cheats     BOOLEAN     NOT NULL DEFAULT FALSE,
    is_recommended      BOOLEAN     NOT NULL DEFAULT FALSE,
    priority            INTEGER     NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_retrogame_controller_configs (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id    TEXT        NOT NULL DEFAULT 'primary',
    user_id              TEXT        NOT NULL,
    config_name          TEXT        NOT NULL,
    platform             TEXT        NOT NULL,
    controller_type      TEXT,
    button_mapping       JSONB       NOT NULL DEFAULT '{}',
    touch_layout         JSONB,
    analog_sensitivity   FLOAT       NOT NULL DEFAULT 1.0,
    vibration_enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_controller_configs_source_account ON np_retrogame_controller_configs(source_account_id);

CREATE TABLE IF NOT EXISTS np_retrogame_core_installations (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    device_id         TEXT        NOT NULL,
    device_platform   TEXT,
    core_name         TEXT        NOT NULL,
    core_version      TEXT,
    installed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_account_id, user_id, device_id, core_name)
);
CREATE INDEX IF NOT EXISTS idx_np_retrogame_core_installations_source_account ON np_retrogame_core_installations(source_account_id);
