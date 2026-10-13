-- np_podcast_plays: one row per episode play. Created by Go DDL at runtime before boot migrations existed;
-- IF NOT EXISTS keeps an existing install (and its rows) untouched on first boot.

CREATE TABLE IF NOT EXISTS np_podcast_plays (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    episode_id        UUID        NOT NULL REFERENCES np_podcast_episodes(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
