-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only, fixed primary keys so the upgrade pass can find them again.
INSERT INTO np_retrogame_roms (id, rom_file_path, game_title, game_title_normalized, platform)
  VALUES ('00000000-0000-0000-0000-000000000001', '/roms/seed.sfc', 'Seed Game', 'seed game', 'snes');
INSERT INTO np_retrogame_save_states (id, user_id, rom_id, slot, save_state_file_path, emulator_core)
  VALUES ('00000000-0000-0000-0000-000000000002', 'seed-user', '00000000-0000-0000-0000-000000000001', 1, '/saves/seed.state', 'snes9x');
INSERT INTO np_retrogame_play_sessions (id, user_id, rom_id, platform, emulator_core)
  VALUES ('00000000-0000-0000-0000-000000000003', 'seed-user', '00000000-0000-0000-0000-000000000001', 'snes', 'snes9x');
INSERT INTO np_retrogame_emulator_cores (id, core_name, display_name, platform, version)
  VALUES ('00000000-0000-0000-0000-000000000004', 'snes9x', 'Snes9x', 'snes', '1.0');
INSERT INTO np_retrogame_controller_configs (id, user_id, config_name, controller_type)
  VALUES ('00000000-0000-0000-0000-000000000005', 'seed-user', 'default', 'gamepad');
INSERT INTO np_retrogame_core_installations (id, user_id, device_id, device_platform, core_name, core_version)
  VALUES ('00000000-0000-0000-0000-000000000006', 'seed-user', 'seed-device', 'web', 'snes9x', '1.0');
