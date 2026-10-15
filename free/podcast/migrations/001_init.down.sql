-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
DROP TABLE IF EXISTS public.np_podcast_playback_positions;
DROP TABLE IF EXISTS public.np_podcast_subscriptions;
DROP TABLE IF EXISTS public.np_podcast_episodes;
DROP TABLE IF EXISTS public.np_podcast_podcasts;
DROP TABLE IF EXISTS public.np_podcast_categories;
