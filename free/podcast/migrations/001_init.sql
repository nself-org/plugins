-- plugin-podcast: Podcast management schema
-- Convention Wall: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Tables derived from db.go InitSchema (code wins)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_podcast_podcasts (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    title             TEXT        NOT NULL,
    description       TEXT,
    author            TEXT,
    image_url         TEXT,
    language          TEXT        NOT NULL DEFAULT 'en',
    is_public         BOOLEAN     NOT NULL DEFAULT FALSE,
    episode_count     INT         NOT NULL DEFAULT 0,
    subscriber_count  INT         NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_podcast_podcasts_source_account ON np_podcast_podcasts(source_account_id);

CREATE TABLE IF NOT EXISTS np_podcast_episodes (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    podcast_id        UUID        NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    title             TEXT        NOT NULL,
    description       TEXT,
    audio_url         TEXT,
    duration_seconds  INT         NOT NULL DEFAULT 0,
    season            INT         NOT NULL DEFAULT 1,
    episode_number    INT         NOT NULL DEFAULT 0,
    is_published      BOOLEAN     NOT NULL DEFAULT FALSE,
    play_count        INT         NOT NULL DEFAULT 0,
    published_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_source_account ON np_podcast_episodes(source_account_id);

CREATE TABLE IF NOT EXISTS np_podcast_subscriptions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    podcast_id        UUID        NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (podcast_id, source_account_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_np_podcast_subscriptions_source_account ON np_podcast_subscriptions(source_account_id);

CREATE TABLE IF NOT EXISTS np_podcast_playback_positions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    episode_id        UUID        NOT NULL REFERENCES np_podcast_episodes(id) ON DELETE CASCADE,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    position_seconds  INT         NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(episode_id, source_account_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_np_podcast_playback_source_account ON np_podcast_playback_positions(source_account_id);

CREATE TABLE IF NOT EXISTS np_podcast_categories (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    name              TEXT        NOT NULL,
    slug              TEXT        NOT NULL,
    parent_id         UUID        REFERENCES np_podcast_categories(id) ON DELETE SET NULL,
    UNIQUE(source_account_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_np_podcast_categories_source_account ON np_podcast_categories(source_account_id);
