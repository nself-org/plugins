-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade. Parent rows first. Test data only.
INSERT INTO np_podcast_podcasts (id, title) VALUES ('00000000-0000-0000-0000-0000000000a1', 'seed podcast');
INSERT INTO np_podcast_episodes (id, podcast_id, title) VALUES ('00000000-0000-0000-0000-0000000000b1', '00000000-0000-0000-0000-0000000000a1', 'seed episode');
INSERT INTO np_podcast_subscriptions (podcast_id, user_id) VALUES ('00000000-0000-0000-0000-0000000000a1', 'seed-user');
INSERT INTO np_podcast_plays (episode_id) VALUES ('00000000-0000-0000-0000-0000000000b1');
