/**
 * Retro Gaming Database Operations
 * Complete CRUD operations for ROMs, save states, play sessions, emulator cores,
 * controller configs, and core installations
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  RomRecord,
  SaveStateRecord,
  PlaySessionRecord,
  EmulatorCoreRecord,
  ControllerConfigRecord,
  CoreInstallationRecord,
  RomStats,
} from './types.js';

const logger = createLogger('retro-gaming:db');

// Platform-to-recommended-core mapping for seeding
const DEFAULT_CORES: Array<Omit<EmulatorCoreRecord, 'id' | 'created_at' | 'updated_at'>> = [
  {
    core_name: 'nestopia',
    display_name: 'Nestopia UE',
    platform: 'nes',
    core_wasm_path: '/cores/nestopia_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '1.52.0',
    license: 'GPL-2.0',
    author: 'Nestopia UE Team',
    homepage_url: 'https://github.com/libretro/nestopia',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'snes9x',
    display_name: 'Snes9x',
    platform: 'snes',
    core_wasm_path: '/cores/snes9x_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '1.62.3',
    license: 'Non-commercial',
    author: 'Snes9x Team',
    homepage_url: 'https://github.com/libretro/snes9x',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'gambatte',
    display_name: 'Gambatte',
    platform: 'gb',
    core_wasm_path: '/cores/gambatte_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '0.5.0',
    license: 'GPL-2.0',
    author: 'sinamas',
    homepage_url: 'https://github.com/libretro/gambatte-libretro',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'mgba',
    display_name: 'mGBA',
    platform: 'gba',
    core_wasm_path: '/cores/mgba_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '0.10.3',
    license: 'MPL-2.0',
    author: 'endrift',
    homepage_url: 'https://github.com/libretro/mgba',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'genesis_plus_gx',
    display_name: 'Genesis Plus GX',
    platform: 'genesis',
    core_wasm_path: '/cores/genesis_plus_gx_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '1.7.4',
    license: 'Non-commercial',
    author: 'ekeeke',
    homepage_url: 'https://github.com/libretro/Genesis-Plus-GX',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'mupen64plus',
    display_name: 'Mupen64Plus-Next',
    platform: 'n64',
    core_wasm_path: '/cores/mupen64plus_next_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '2.5.9',
    license: 'GPL-2.0',
    author: 'Mupen64Plus Team',
    homepage_url: 'https://github.com/libretro/mupen64plus-libretro-nx',
    supports_save_states: true,
    supports_rewind: false,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'pcsx_rearmed',
    display_name: 'PCSX ReARMed',
    platform: 'ps1',
    core_wasm_path: '/cores/pcsx_rearmed_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '23',
    license: 'GPL-2.0',
    author: 'notaz',
    homepage_url: 'https://github.com/libretro/pcsx_rearmed',
    supports_save_states: true,
    supports_rewind: false,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'mame2003_plus',
    display_name: 'MAME 2003-Plus',
    platform: 'arcade',
    core_wasm_path: '/cores/mame2003_plus_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '2003+',
    license: 'MAME',
    author: 'MAME Team',
    homepage_url: 'https://github.com/libretro/mame2003-plus-libretro',
    supports_save_states: true,
    supports_rewind: false,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: true,
    priority: 1,
  },
  {
    core_name: 'fceux',
    display_name: 'FCEUmm',
    platform: 'nes',
    core_wasm_path: '/cores/fceumm_libretro.wasm',
    core_wasm_size_bytes: null,
    version: '2.6.0',
    license: 'GPL-2.0',
    author: 'FCEUmm Team',
    homepage_url: 'https://github.com/libretro/libretro-fceumm',
    supports_save_states: true,
    supports_rewind: true,
    supports_fast_forward: true,
    supports_cheats: true,
    default_config: {},
    is_recommended: false,
    priority: 2,
  },
];

export class RetroGamingDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): RetroGamingDatabase {
    return new RetroGamingDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing retro-gaming schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- ROMs
      -- =====================================================================

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

      -- =====================================================================
      -- Save States
      -- =====================================================================

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

      -- =====================================================================
      -- Play Sessions
      -- =====================================================================

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

      -- =====================================================================
      -- Emulator Cores
      -- =====================================================================

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

      -- =====================================================================
      -- Controller Configs
      -- =====================================================================

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

      -- =====================================================================
      -- Core Installations
      -- =====================================================================

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
    `;

    await this.execute(schema);
    logger.info('Retro-gaming schema initialized successfully');
  }

  // =========================================================================
  // Seed Default Cores
  // =========================================================================

  async seedDefaultCores(): Promise<number> {
    let seeded = 0;
    for (const core of DEFAULT_CORES) {
      const result = await this.query<EmulatorCoreRecord>(
        `INSERT INTO np_retrogame_emulator_cores (
          core_name, display_name, platform, core_wasm_path, core_wasm_size_bytes,
          version, license, author, homepage_url,
          supports_save_states, supports_rewind, supports_fast_forward, supports_cheats,
          default_config, is_recommended, priority
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
        ON CONFLICT (core_name, platform) DO UPDATE SET
          display_name = EXCLUDED.display_name,
          core_wasm_path = EXCLUDED.core_wasm_path,
          version = EXCLUDED.version,
          license = EXCLUDED.license,
          author = EXCLUDED.author,
          homepage_url = EXCLUDED.homepage_url,
          supports_save_states = EXCLUDED.supports_save_states,
          supports_rewind = EXCLUDED.supports_rewind,
          supports_fast_forward = EXCLUDED.supports_fast_forward,
          supports_cheats = EXCLUDED.supports_cheats,
          is_recommended = EXCLUDED.is_recommended,
          priority = EXCLUDED.priority,
          updated_at = NOW()
        RETURNING *`,
        [
          core.core_name, core.display_name, core.platform,
          core.core_wasm_path, core.core_wasm_size_bytes,
          core.version, core.license, core.author, core.homepage_url,
          core.supports_save_states, core.supports_rewind,
          core.supports_fast_forward, core.supports_cheats,
          JSON.stringify(core.default_config), core.is_recommended, core.priority,
        ]
      );
      if (result.rows.length > 0) {
        seeded++;
      }
    }
    logger.info(`Seeded ${seeded} default emulator cores`);
    return seeded;
  }

  // =========================================================================
  // ROM Operations
  // =========================================================================

  private normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9\s]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
  }

  async createRom(rom: Omit<RomRecord, 'id' | 'created_at' | 'updated_at' | 'game_title_normalized' | 'play_count' | 'last_played_at' | 'favorite'>): Promise<RomRecord> {
    const titleNormalized = this.normalizeTitle(rom.game_title as string);
    const result = await this.query<RomRecord>(
      `INSERT INTO np_retrogame_roms (
        source_account_id, rom_file_path, rom_file_size_bytes, rom_file_hash,
        game_title, game_title_normalized, platform, region, release_year,
        genre, publisher, developer, igdb_id, moby_games_id,
        box_art_url, box_art_local_path, screenshot_urls, screenshot_local_paths,
        description, description_source, recommended_core, core_overrides,
        user_rating, scan_source, added_by_user_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
      RETURNING *`,
      [
        this.sourceAccountId, rom.rom_file_path, rom.rom_file_size_bytes, rom.rom_file_hash,
        rom.game_title, titleNormalized, rom.platform, rom.region, rom.release_year,
        rom.genre, rom.publisher, rom.developer, rom.igdb_id, rom.moby_games_id,
        rom.box_art_url, rom.box_art_local_path, rom.screenshot_urls ?? [], rom.screenshot_local_paths ?? [],
        rom.description, rom.description_source, rom.recommended_core, JSON.stringify(rom.core_overrides ?? {}),
        rom.user_rating, rom.scan_source, rom.added_by_user_id,
      ]
    );
    return result.rows[0];
  }

  async getRom(id: string): Promise<RomRecord | null> {
    const result = await this.query<RomRecord>(
      `SELECT * FROM np_retrogame_roms WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getRomByFilePath(filePath: string): Promise<RomRecord | null> {
    const result = await this.query<RomRecord>(
      `SELECT * FROM np_retrogame_roms WHERE rom_file_path = $1 AND source_account_id = $2`,
      [filePath, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listRoms(filters: {
    platform?: string;
    genre?: string;
    favorite?: boolean;
    search?: string;
    sort?: string;
    limit?: number;
    offset?: number;
  }): Promise<RomRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.platform) {
      conditions.push(`platform = $${paramIndex}`);
      values.push(filters.platform);
      paramIndex++;
    }

    if (filters.genre) {
      conditions.push(`genre = $${paramIndex}`);
      values.push(filters.genre);
      paramIndex++;
    }

    if (filters.favorite !== undefined) {
      conditions.push(`favorite = $${paramIndex}`);
      values.push(filters.favorite);
      paramIndex++;
    }

    if (filters.search) {
      conditions.push(`game_title_normalized LIKE $${paramIndex}`);
      values.push(`%${this.normalizeTitle(filters.search)}%`);
      paramIndex++;
    }

    let orderBy = 'game_title ASC';
    if (filters.sort === 'recent') {
      orderBy = 'last_played_at DESC NULLS LAST';
    } else if (filters.sort === 'added') {
      orderBy = 'created_at DESC';
    } else if (filters.sort === 'most_played') {
      orderBy = 'play_count DESC';
    } else if (filters.sort === 'platform') {
      orderBy = 'platform ASC, game_title ASC';
    }

    const limit = filters.limit ?? 100;
    const offset = filters.offset ?? 0;

    const result = await this.query<RomRecord>(
      `SELECT * FROM np_retrogame_roms
       WHERE ${conditions.join(' AND ')}
       ORDER BY ${orderBy}
       LIMIT ${limit} OFFSET ${offset}`,
      values
    );

    return result.rows;
  }

  async updateRom(id: string, updates: Record<string, unknown>): Promise<RomRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'game_title', 'platform', 'region', 'release_year', 'genre',
      'publisher', 'developer', 'igdb_id', 'moby_games_id',
      'box_art_url', 'box_art_local_path', 'description',
      'description_source', 'recommended_core', 'user_rating', 'favorite',
    ];

    const jsonFields = ['core_overrides'];
    const arrayFields = ['screenshot_urls', 'screenshot_local_paths'];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      } else if (jsonFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}::jsonb`);
        values.push(JSON.stringify(value));
        paramIndex++;
      } else if (arrayFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    // If game_title was updated, also update normalized
    if (updates.game_title && typeof updates.game_title === 'string') {
      fields.push(`game_title_normalized = $${paramIndex}`);
      values.push(this.normalizeTitle(updates.game_title));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getRom(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<RomRecord>(
      `UPDATE np_retrogame_roms
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteRom(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_retrogame_roms WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async incrementRomPlayCount(id: string): Promise<void> {
    await this.execute(
      `UPDATE np_retrogame_roms
       SET play_count = play_count + 1, last_played_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Save State Operations
  // =========================================================================

  async createSaveState(saveState: Omit<SaveStateRecord, 'id' | 'created_at' | 'updated_at'>): Promise<SaveStateRecord> {
    const result = await this.query<SaveStateRecord>(
      `INSERT INTO np_retrogame_save_states (
        user_id, rom_id, source_account_id, slot,
        save_state_file_path, save_state_file_size_bytes,
        screenshot_url, screenshot_local_path,
        emulator_core, emulator_version, description, play_time_seconds
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      ON CONFLICT (source_account_id, user_id, rom_id, slot) DO UPDATE SET
        save_state_file_path = EXCLUDED.save_state_file_path,
        save_state_file_size_bytes = EXCLUDED.save_state_file_size_bytes,
        screenshot_url = EXCLUDED.screenshot_url,
        screenshot_local_path = EXCLUDED.screenshot_local_path,
        emulator_core = EXCLUDED.emulator_core,
        emulator_version = EXCLUDED.emulator_version,
        description = EXCLUDED.description,
        play_time_seconds = EXCLUDED.play_time_seconds,
        updated_at = NOW()
      RETURNING *`,
      [
        saveState.user_id, saveState.rom_id, this.sourceAccountId, saveState.slot,
        saveState.save_state_file_path, saveState.save_state_file_size_bytes,
        saveState.screenshot_url, saveState.screenshot_local_path,
        saveState.emulator_core, saveState.emulator_version,
        saveState.description, saveState.play_time_seconds,
      ]
    );
    return result.rows[0];
  }

  async listSaveStates(romId: string): Promise<SaveStateRecord[]> {
    const result = await this.query<SaveStateRecord>(
      `SELECT * FROM np_retrogame_save_states
       WHERE rom_id = $1 AND source_account_id = $2
       ORDER BY slot ASC`,
      [romId, this.sourceAccountId]
    );
    return result.rows;
  }

  async getSaveState(romId: string, slot: number): Promise<SaveStateRecord | null> {
    const result = await this.query<SaveStateRecord>(
      `SELECT * FROM np_retrogame_save_states
       WHERE rom_id = $1 AND slot = $2 AND source_account_id = $3`,
      [romId, slot, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getSaveStateById(id: string): Promise<SaveStateRecord | null> {
    const result = await this.query<SaveStateRecord>(
      `SELECT * FROM np_retrogame_save_states
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async deleteSaveState(romId: string, slot: number): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_retrogame_save_states
       WHERE rom_id = $1 AND slot = $2 AND source_account_id = $3`,
      [romId, slot, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Play Session Operations
  // =========================================================================

  async startPlaySession(session: Omit<PlaySessionRecord, 'id' | 'ended_at' | 'duration_seconds' | 'save_state_id' | 'auto_save_created' | 'created_at'>): Promise<PlaySessionRecord> {
    const result = await this.query<PlaySessionRecord>(
      `INSERT INTO np_retrogame_play_sessions (
        user_id, rom_id, source_account_id, platform, device_id,
        emulator_core, started_at, controller_type
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
      RETURNING *`,
      [
        session.user_id, session.rom_id, this.sourceAccountId,
        session.platform, session.device_id, session.emulator_core,
        session.controller_type,
      ]
    );
    return result.rows[0];
  }

  async endPlaySession(sessionId: string, saveStateId: string | null, autoSaveCreated: boolean): Promise<PlaySessionRecord | null> {
    const result = await this.query<PlaySessionRecord>(
      `UPDATE np_retrogame_play_sessions
       SET ended_at = NOW(),
           duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER,
           save_state_id = $1,
           auto_save_created = $2
       WHERE id = $3 AND source_account_id = $4 AND ended_at IS NULL
       RETURNING *`,
      [saveStateId, autoSaveCreated, sessionId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPlaySession(sessionId: string): Promise<PlaySessionRecord | null> {
    const result = await this.query<PlaySessionRecord>(
      `SELECT * FROM np_retrogame_play_sessions
       WHERE id = $1 AND source_account_id = $2`,
      [sessionId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listRecentSessions(limit = 20): Promise<(PlaySessionRecord & { game_title: string })[]> {
    const result = await this.query<PlaySessionRecord & { game_title: string }>(
      `SELECT s.*, r.game_title
       FROM np_retrogame_play_sessions s
       JOIN np_retrogame_roms r ON s.rom_id = r.id
       WHERE s.source_account_id = $1
       ORDER BY s.started_at DESC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // Emulator Core Operations
  // =========================================================================

  async listCores(platform?: string): Promise<EmulatorCoreRecord[]> {
    if (platform) {
      const result = await this.query<EmulatorCoreRecord>(
        `SELECT * FROM np_retrogame_emulator_cores
         WHERE platform = $1
         ORDER BY is_recommended DESC, priority ASC, display_name ASC`,
        [platform]
      );
      return result.rows;
    }

    const result = await this.query<EmulatorCoreRecord>(
      `SELECT * FROM np_retrogame_emulator_cores
       ORDER BY platform ASC, is_recommended DESC, priority ASC, display_name ASC`
    );
    return result.rows;
  }

  async getRecommendedCore(platform: string): Promise<EmulatorCoreRecord | null> {
    const result = await this.query<EmulatorCoreRecord>(
      `SELECT * FROM np_retrogame_emulator_cores
       WHERE platform = $1 AND is_recommended = true
       ORDER BY priority ASC
       LIMIT 1`,
      [platform]
    );
    return result.rows[0] ?? null;
  }

  async getCoreByName(coreName: string): Promise<EmulatorCoreRecord | null> {
    const result = await this.query<EmulatorCoreRecord>(
      `SELECT * FROM np_retrogame_emulator_cores
       WHERE core_name = $1
       ORDER BY priority ASC
       LIMIT 1`,
      [coreName]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Core Installation Operations
  // =========================================================================

  async recordCoreInstallation(installation: Omit<CoreInstallationRecord, 'id' | 'installed_at' | 'last_used_at'>): Promise<CoreInstallationRecord> {
    const result = await this.query<CoreInstallationRecord>(
      `INSERT INTO np_retrogame_core_installations (
        user_id, source_account_id, device_id, device_platform, core_name, core_version
      ) VALUES ($1, $2, $3, $4, $5, $6)
      ON CONFLICT (source_account_id, user_id, device_id, core_name) DO UPDATE SET
        core_version = EXCLUDED.core_version,
        installed_at = NOW()
      RETURNING *`,
      [
        installation.user_id, this.sourceAccountId,
        installation.device_id, installation.device_platform,
        installation.core_name, installation.core_version,
      ]
    );
    return result.rows[0];
  }

  async listInstalledCores(deviceId?: string): Promise<CoreInstallationRecord[]> {
    if (deviceId) {
      const result = await this.query<CoreInstallationRecord>(
        `SELECT * FROM np_retrogame_core_installations
         WHERE source_account_id = $1 AND device_id = $2
         ORDER BY core_name ASC`,
        [this.sourceAccountId, deviceId]
      );
      return result.rows;
    }

    const result = await this.query<CoreInstallationRecord>(
      `SELECT * FROM np_retrogame_core_installations
       WHERE source_account_id = $1
       ORDER BY device_id ASC, core_name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async updateCoreLastUsed(userId: string, deviceId: string, coreName: string): Promise<void> {
    await this.execute(
      `UPDATE np_retrogame_core_installations
       SET last_used_at = NOW()
       WHERE source_account_id = $1 AND user_id = $2 AND device_id = $3 AND core_name = $4`,
      [this.sourceAccountId, userId, deviceId, coreName]
    );
  }

  // =========================================================================
  // Controller Config Operations
  // =========================================================================

  async createControllerConfig(config: Omit<ControllerConfigRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ControllerConfigRecord> {
    const result = await this.query<ControllerConfigRecord>(
      `INSERT INTO np_retrogame_controller_configs (
        source_account_id, user_id, config_name, platform, controller_type,
        button_mapping, touch_layout, analog_sensitivity, vibration_enabled
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      ON CONFLICT (source_account_id, user_id, config_name) DO UPDATE SET
        platform = EXCLUDED.platform,
        controller_type = EXCLUDED.controller_type,
        button_mapping = EXCLUDED.button_mapping,
        touch_layout = EXCLUDED.touch_layout,
        analog_sensitivity = EXCLUDED.analog_sensitivity,
        vibration_enabled = EXCLUDED.vibration_enabled,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, config.user_id, config.config_name,
        config.platform, config.controller_type,
        JSON.stringify(config.button_mapping), JSON.stringify(config.touch_layout),
        config.analog_sensitivity, config.vibration_enabled,
      ]
    );
    return result.rows[0];
  }

  async listControllerConfigs(userId?: string): Promise<ControllerConfigRecord[]> {
    if (userId) {
      const result = await this.query<ControllerConfigRecord>(
        `SELECT * FROM np_retrogame_controller_configs
         WHERE source_account_id = $1 AND user_id = $2
         ORDER BY config_name ASC`,
        [this.sourceAccountId, userId]
      );
      return result.rows;
    }

    const result = await this.query<ControllerConfigRecord>(
      `SELECT * FROM np_retrogame_controller_configs
       WHERE source_account_id = $1
       ORDER BY user_id ASC, config_name ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async deleteControllerConfig(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_retrogame_controller_configs
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<RomStats> {
    const result = await this.query<{
      total_roms: string;
      total_platforms: string;
      total_play_sessions: string;
      total_play_time_seconds: string;
      total_save_states: string;
      total_favorites: string;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_retrogame_roms WHERE source_account_id = $1) as total_roms,
        (SELECT COUNT(DISTINCT platform) FROM np_retrogame_roms WHERE source_account_id = $1) as total_platforms,
        (SELECT COUNT(*) FROM np_retrogame_play_sessions WHERE source_account_id = $1) as total_play_sessions,
        (SELECT COALESCE(SUM(duration_seconds), 0) FROM np_retrogame_play_sessions WHERE source_account_id = $1 AND duration_seconds IS NOT NULL) as total_play_time_seconds,
        (SELECT COUNT(*) FROM np_retrogame_save_states WHERE source_account_id = $1) as total_save_states,
        (SELECT COUNT(*) FROM np_retrogame_roms WHERE source_account_id = $1 AND favorite = true) as total_favorites`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // ROMs by platform
    const platformResult = await this.query<{ platform: string; count: string }>(
      `SELECT platform, COUNT(*) as count
       FROM np_retrogame_roms
       WHERE source_account_id = $1
       GROUP BY platform
       ORDER BY count DESC`,
      [this.sourceAccountId]
    );

    // Most played
    const mostPlayedResult = await this.query<{ rom_id: string; game_title: string; play_count: string }>(
      `SELECT id as rom_id, game_title, play_count
       FROM np_retrogame_roms
       WHERE source_account_id = $1 AND play_count > 0
       ORDER BY play_count DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    return {
      total_roms: parseInt(row.total_roms, 10),
      total_platforms: parseInt(row.total_platforms, 10),
      total_play_sessions: parseInt(row.total_play_sessions, 10),
      total_play_time_seconds: parseInt(row.total_play_time_seconds, 10),
      total_save_states: parseInt(row.total_save_states, 10),
      total_favorites: parseInt(row.total_favorites, 10),
      roms_by_platform: platformResult.rows.map(r => ({
        platform: r.platform,
        count: parseInt(r.count, 10),
      })),
      most_played: mostPlayedResult.rows.map(r => ({
        rom_id: r.rom_id,
        game_title: r.game_title,
        play_count: parseInt(r.play_count, 10),
      })),
    };
  }
}
