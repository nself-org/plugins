-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
DROP TABLE IF EXISTS np_retrogame_core_installations;
DROP TABLE IF EXISTS np_retrogame_controller_configs;
DROP TABLE IF EXISTS np_retrogame_emulator_cores;
DROP TABLE IF EXISTS np_retrogame_play_sessions;
DROP TABLE IF EXISTS np_retrogame_save_states;
DROP TABLE IF EXISTS np_retrogame_roms;
