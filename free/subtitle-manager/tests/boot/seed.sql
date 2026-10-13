-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only, fixed primary keys so the upgrade pass can find them again.
INSERT INTO np_subtmgr_subtitles (id, media_id, media_type, language, file_path, source)
  VALUES ('00000000-0000-0000-0000-000000000001', 'seed-media', 'movie', 'en', '/subs/seed.srt', 'opensubtitles');
INSERT INTO np_subtmgr_downloads (id, subtitle_id, media_id, media_type, language, file_path)
  VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'seed-media', 'movie', 'en', '/subs/seed.srt');
INSERT INTO np_subtmgr_qc_results (id, download_id, status)
  VALUES ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', 'pass');
