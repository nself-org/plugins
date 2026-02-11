/**
 * LiveKit Database Operations
 * Complete CRUD operations for LiveKit rooms, participants, egress, tokens, and quality metrics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  LiveKitRoomRecord,
  CreateRoomRequest,
  UpdateRoomRequest,
  LiveKitParticipantRecord,
  CreateParticipantRequest,
  UpdateParticipantRequest,
  LiveKitEgressJobRecord,
  CreateEgressJobRequest,
  UpdateEgressJobRequest,
  LiveKitTokenRecord,
  LiveKitQualityMetricRecord,
  CreateQualityMetricRequest,
  LiveKitStats,
  RoomStatus,
  EgressStatus,
  ParticipantStatus,
} from './types.js';

const logger = createLogger('livekit:db');

export class LiveKitDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): LiveKitDatabase {
    return new LiveKitDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing LiveKit schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- LiveKit Rooms
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_rooms (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        livekit_room_name VARCHAR(255) NOT NULL,
        livekit_room_sid VARCHAR(255),
        room_type VARCHAR(50) NOT NULL,
        max_participants INTEGER DEFAULT 100,
        empty_timeout INTEGER DEFAULT 300,
        call_id UUID,
        stream_id UUID,
        status VARCHAR(50) NOT NULL DEFAULT 'creating',
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        activated_at TIMESTAMPTZ,
        closed_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}'::jsonb,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, livekit_room_name)
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_account ON nchat_livekit_rooms(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_name ON nchat_livekit_rooms(livekit_room_name);
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_sid ON nchat_livekit_rooms(livekit_room_sid);
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_status ON nchat_livekit_rooms(status);
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_call ON nchat_livekit_rooms(call_id) WHERE call_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_livekit_rooms_stream ON nchat_livekit_rooms(stream_id) WHERE stream_id IS NOT NULL;

      -- =====================================================================
      -- LiveKit Participants
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_participants (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID NOT NULL REFERENCES nchat_livekit_rooms(id) ON DELETE CASCADE,
        user_id UUID NOT NULL,
        livekit_participant_sid VARCHAR(255),
        livekit_identity VARCHAR(255) NOT NULL,
        display_name VARCHAR(255),
        metadata JSONB DEFAULT '{}'::jsonb,
        status VARCHAR(50) NOT NULL DEFAULT 'joining',
        camera_enabled BOOLEAN DEFAULT false,
        microphone_enabled BOOLEAN DEFAULT false,
        screen_share_enabled BOOLEAN DEFAULT false,
        last_bitrate_kbps INTEGER,
        last_latency_ms INTEGER,
        last_packet_loss_pct DECIMAL(5,2),
        joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        left_at TIMESTAMPTZ,
        total_duration_seconds INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, room_id, user_id)
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_participants_account ON nchat_livekit_participants(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_participants_room ON nchat_livekit_participants(room_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_participants_user ON nchat_livekit_participants(user_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_participants_sid ON nchat_livekit_participants(livekit_participant_sid);
      CREATE INDEX IF NOT EXISTS idx_livekit_participants_status ON nchat_livekit_participants(status);

      -- =====================================================================
      -- LiveKit Egress Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_egress_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID NOT NULL REFERENCES nchat_livekit_rooms(id) ON DELETE CASCADE,
        recording_id UUID,
        livekit_egress_id VARCHAR(255) NOT NULL,
        egress_type VARCHAR(50) NOT NULL,
        output_type VARCHAR(50) NOT NULL,
        config JSONB NOT NULL DEFAULT '{}'::jsonb,
        status VARCHAR(50) NOT NULL DEFAULT 'pending',
        file_url TEXT,
        file_size_bytes BIGINT,
        duration_seconds INTEGER,
        playlist_url TEXT,
        error_message TEXT,
        started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        ended_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, livekit_egress_id)
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_account ON nchat_livekit_egress_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_room ON nchat_livekit_egress_jobs(room_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_recording ON nchat_livekit_egress_jobs(recording_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_id ON nchat_livekit_egress_jobs(livekit_egress_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_status ON nchat_livekit_egress_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_livekit_egress_type ON nchat_livekit_egress_jobs(egress_type, output_type);

      -- =====================================================================
      -- LiveKit Tokens
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_tokens (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID NOT NULL REFERENCES nchat_livekit_rooms(id) ON DELETE CASCADE,
        user_id UUID NOT NULL,
        token_hash VARCHAR(255) NOT NULL,
        grants JSONB NOT NULL DEFAULT '{}'::jsonb,
        issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        expires_at TIMESTAMPTZ NOT NULL,
        revoked_at TIMESTAMPTZ,
        revoked_by UUID,
        revoke_reason TEXT,
        first_used_at TIMESTAMPTZ,
        last_used_at TIMESTAMPTZ,
        use_count INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_tokens_account ON nchat_livekit_tokens(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_tokens_room ON nchat_livekit_tokens(room_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_tokens_user ON nchat_livekit_tokens(user_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_tokens_expires ON nchat_livekit_tokens(expires_at);
      CREATE INDEX IF NOT EXISTS idx_livekit_tokens_revoked ON nchat_livekit_tokens(revoked_at) WHERE revoked_at IS NOT NULL;

      -- =====================================================================
      -- LiveKit Quality Metrics
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_quality_metrics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID NOT NULL REFERENCES nchat_livekit_rooms(id) ON DELETE CASCADE,
        participant_id UUID REFERENCES nchat_livekit_participants(id) ON DELETE CASCADE,
        metric_type VARCHAR(50) NOT NULL,
        bitrate_kbps INTEGER,
        latency_ms INTEGER,
        jitter_ms INTEGER,
        packet_loss_pct DECIMAL(5,2),
        resolution VARCHAR(20),
        fps INTEGER,
        audio_level INTEGER,
        connection_type VARCHAR(50),
        turn_server VARCHAR(255),
        metadata JSONB DEFAULT '{}'::jsonb,
        recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_quality_account ON nchat_livekit_quality_metrics(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_quality_room ON nchat_livekit_quality_metrics(room_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_quality_participant ON nchat_livekit_quality_metrics(participant_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_quality_type ON nchat_livekit_quality_metrics(metric_type);
      CREATE INDEX IF NOT EXISTS idx_livekit_quality_time ON nchat_livekit_quality_metrics(recorded_at DESC);

      -- =====================================================================
      -- LiveKit Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_livekit_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_livekit_webhook_events_account ON nchat_livekit_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_livekit_webhook_events_type ON nchat_livekit_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_livekit_webhook_events_processed ON nchat_livekit_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_livekit_webhook_events_created ON nchat_livekit_webhook_events(created_at);
    `;

    await this.db.execute(schema);
    logger.info('LiveKit schema initialized successfully');
  }

  // =========================================================================
  // Room Management
  // =========================================================================

  async createRoom(request: CreateRoomRequest): Promise<LiveKitRoomRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_livekit_rooms (
        source_account_id, livekit_room_name, room_type, max_participants,
        empty_timeout, call_id, stream_id, status, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'creating', $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.roomName,
        request.roomType,
        request.maxParticipants ?? 100,
        request.emptyTimeout ?? 300,
        request.callId ?? null,
        request.streamId ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as LiveKitRoomRecord;
  }

  async getRoom(roomId: string): Promise<LiveKitRoomRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_rooms WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, roomId]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitRoomRecord | null;
  }

  async getRoomByName(roomName: string): Promise<LiveKitRoomRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_rooms WHERE source_account_id = $1 AND livekit_room_name = $2',
      [this.sourceAccountId, roomName]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitRoomRecord | null;
  }

  async listRooms(options: { status?: RoomStatus; roomType?: string; limit?: number; offset?: number } = {}): Promise<LiveKitRoomRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(options.status);
    }
    if (options.roomType) {
      conditions.push(`room_type = $${paramIndex++}`);
      params.push(options.roomType);
    }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_livekit_rooms
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as LiveKitRoomRecord[];
  }

  async updateRoom(roomId: string, updates: UpdateRoomRequest): Promise<LiveKitRoomRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, roomId];
    let paramIndex = 3;

    if (updates.livekitRoomSid !== undefined) { sets.push(`livekit_room_sid = $${paramIndex++}`); params.push(updates.livekitRoomSid); }
    if (updates.status !== undefined) { sets.push(`status = $${paramIndex++}`); params.push(updates.status); }
    if (updates.maxParticipants !== undefined) { sets.push(`max_participants = $${paramIndex++}`); params.push(updates.maxParticipants); }
    if (updates.emptyTimeout !== undefined) { sets.push(`empty_timeout = $${paramIndex++}`); params.push(updates.emptyTimeout); }
    if (updates.activatedAt !== undefined) { sets.push(`activated_at = $${paramIndex++}`); params.push(updates.activatedAt); }
    if (updates.closedAt !== undefined) { sets.push(`closed_at = $${paramIndex++}`); params.push(updates.closedAt); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getRoom(roomId);

    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_livekit_rooms SET ${sets.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as LiveKitRoomRecord | null;
  }

  async deleteRoom(roomId: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM nchat_livekit_rooms WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, roomId]
    );
    return count > 0;
  }

  async closeRoom(roomId: string): Promise<LiveKitRoomRecord | null> {
    return this.updateRoom(roomId, { status: 'closed', closedAt: new Date().toISOString() });
  }

  // =========================================================================
  // Participant Management
  // =========================================================================

  async createParticipant(request: CreateParticipantRequest): Promise<LiveKitParticipantRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_livekit_participants (
        source_account_id, room_id, user_id, livekit_identity,
        display_name, metadata, status
      ) VALUES ($1, $2, $3, $4, $5, $6, 'joining')
      ON CONFLICT (source_account_id, room_id, user_id) DO UPDATE SET
        livekit_identity = EXCLUDED.livekit_identity,
        display_name = EXCLUDED.display_name,
        status = 'joining',
        left_at = NULL,
        joined_at = NOW(),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.roomId,
        request.userId,
        request.livekitIdentity,
        request.displayName ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as LiveKitParticipantRecord;
  }

  async getParticipant(participantId: string): Promise<LiveKitParticipantRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_participants WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, participantId]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitParticipantRecord | null;
  }

  async listParticipants(roomId: string, status?: ParticipantStatus): Promise<LiveKitParticipantRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'room_id = $2'];
    const params: unknown[] = [this.sourceAccountId, roomId];

    if (status) {
      conditions.push('status = $3');
      params.push(status);
    }

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_livekit_participants
       WHERE ${conditions.join(' AND ')}
       ORDER BY joined_at DESC`,
      params
    );
    return result.rows as unknown as LiveKitParticipantRecord[];
  }

  async updateParticipant(participantId: string, updates: UpdateParticipantRequest): Promise<LiveKitParticipantRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, participantId];
    let paramIndex = 3;

    if (updates.livekitParticipantSid !== undefined) { sets.push(`livekit_participant_sid = $${paramIndex++}`); params.push(updates.livekitParticipantSid); }
    if (updates.status !== undefined) { sets.push(`status = $${paramIndex++}`); params.push(updates.status); }
    if (updates.displayName !== undefined) { sets.push(`display_name = $${paramIndex++}`); params.push(updates.displayName); }
    if (updates.cameraEnabled !== undefined) { sets.push(`camera_enabled = $${paramIndex++}`); params.push(updates.cameraEnabled); }
    if (updates.microphoneEnabled !== undefined) { sets.push(`microphone_enabled = $${paramIndex++}`); params.push(updates.microphoneEnabled); }
    if (updates.screenShareEnabled !== undefined) { sets.push(`screen_share_enabled = $${paramIndex++}`); params.push(updates.screenShareEnabled); }
    if (updates.lastBitrateKbps !== undefined) { sets.push(`last_bitrate_kbps = $${paramIndex++}`); params.push(updates.lastBitrateKbps); }
    if (updates.lastLatencyMs !== undefined) { sets.push(`last_latency_ms = $${paramIndex++}`); params.push(updates.lastLatencyMs); }
    if (updates.lastPacketLossPct !== undefined) { sets.push(`last_packet_loss_pct = $${paramIndex++}`); params.push(updates.lastPacketLossPct); }
    if (updates.leftAt !== undefined) { sets.push(`left_at = $${paramIndex++}`); params.push(updates.leftAt); }
    if (updates.totalDurationSeconds !== undefined) { sets.push(`total_duration_seconds = $${paramIndex++}`); params.push(updates.totalDurationSeconds); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getParticipant(participantId);

    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_livekit_participants SET ${sets.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as LiveKitParticipantRecord | null;
  }

  async removeParticipant(participantId: string): Promise<boolean> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_livekit_participants
       SET status = 'disconnected', left_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, participantId]
    );
    return (result.rows.length ?? 0) > 0;
  }

  // =========================================================================
  // Egress Job Management
  // =========================================================================

  async createEgressJob(request: CreateEgressJobRequest): Promise<LiveKitEgressJobRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_livekit_egress_jobs (
        source_account_id, room_id, recording_id, livekit_egress_id,
        egress_type, output_type, config, status, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.roomId,
        request.recordingId ?? null,
        request.livekitEgressId,
        request.egressType,
        request.outputType,
        JSON.stringify(request.config ?? {}),
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as LiveKitEgressJobRecord;
  }

  async getEgressJob(jobId: string): Promise<LiveKitEgressJobRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_egress_jobs WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, jobId]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitEgressJobRecord | null;
  }

  async getEgressJobByEgressId(egressId: string): Promise<LiveKitEgressJobRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_egress_jobs WHERE source_account_id = $1 AND livekit_egress_id = $2',
      [this.sourceAccountId, egressId]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitEgressJobRecord | null;
  }

  async listEgressJobs(options: { roomId?: string; status?: EgressStatus; egressType?: string; limit?: number; offset?: number } = {}): Promise<LiveKitEgressJobRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.roomId) { conditions.push(`room_id = $${paramIndex++}`); params.push(options.roomId); }
    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }
    if (options.egressType) { conditions.push(`egress_type = $${paramIndex++}`); params.push(options.egressType); }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_livekit_egress_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as LiveKitEgressJobRecord[];
  }

  async updateEgressJob(jobId: string, updates: UpdateEgressJobRequest): Promise<LiveKitEgressJobRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, jobId];
    let paramIndex = 3;

    if (updates.status !== undefined) { sets.push(`status = $${paramIndex++}`); params.push(updates.status); }
    if (updates.fileUrl !== undefined) { sets.push(`file_url = $${paramIndex++}`); params.push(updates.fileUrl); }
    if (updates.fileSizeBytes !== undefined) { sets.push(`file_size_bytes = $${paramIndex++}`); params.push(updates.fileSizeBytes); }
    if (updates.durationSeconds !== undefined) { sets.push(`duration_seconds = $${paramIndex++}`); params.push(updates.durationSeconds); }
    if (updates.playlistUrl !== undefined) { sets.push(`playlist_url = $${paramIndex++}`); params.push(updates.playlistUrl); }
    if (updates.errorMessage !== undefined) { sets.push(`error_message = $${paramIndex++}`); params.push(updates.errorMessage); }
    if (updates.endedAt !== undefined) { sets.push(`ended_at = $${paramIndex++}`); params.push(updates.endedAt); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getEgressJob(jobId);

    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_livekit_egress_jobs SET ${sets.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as LiveKitEgressJobRecord | null;
  }

  async stopEgressJob(egressId: string): Promise<LiveKitEgressJobRecord | null> {
    const job = await this.getEgressJobByEgressId(egressId);
    if (!job) return null;
    return this.updateEgressJob(job.id, { status: 'ending', endedAt: new Date().toISOString() });
  }

  // =========================================================================
  // Token Management
  // =========================================================================

  async createToken(
    roomId: string,
    userId: string,
    tokenHash: string,
    grants: Record<string, unknown>,
    expiresAt: Date,
    metadata?: Record<string, unknown>
  ): Promise<LiveKitTokenRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_livekit_tokens (
        source_account_id, room_id, user_id, token_hash,
        grants, expires_at, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        roomId,
        userId,
        tokenHash,
        JSON.stringify(grants),
        expiresAt.toISOString(),
        JSON.stringify(metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as LiveKitTokenRecord;
  }

  async getToken(tokenId: string): Promise<LiveKitTokenRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_livekit_tokens WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, tokenId]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitTokenRecord | null;
  }

  async listTokens(options: { roomId?: string; userId?: string; limit?: number; offset?: number } = {}): Promise<LiveKitTokenRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.roomId) { conditions.push(`room_id = $${paramIndex++}`); params.push(options.roomId); }
    if (options.userId) { conditions.push(`user_id = $${paramIndex++}`); params.push(options.userId); }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_livekit_tokens
       WHERE ${conditions.join(' AND ')} AND revoked_at IS NULL AND expires_at > NOW()
       ORDER BY issued_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as LiveKitTokenRecord[];
  }

  async revokeToken(tokenId: string, revokedBy: string, reason?: string): Promise<LiveKitTokenRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_livekit_tokens
       SET revoked_at = NOW(), revoked_by = $3, revoke_reason = $4
       WHERE source_account_id = $1 AND id = $2 AND revoked_at IS NULL
       RETURNING *`,
      [this.sourceAccountId, tokenId, revokedBy, reason ?? null]
    );
    return (result.rows[0] ?? null) as unknown as LiveKitTokenRecord | null;
  }

  // =========================================================================
  // Quality Metrics
  // =========================================================================

  async recordQualityMetric(request: CreateQualityMetricRequest): Promise<LiveKitQualityMetricRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_livekit_quality_metrics (
        source_account_id, room_id, participant_id, metric_type,
        bitrate_kbps, latency_ms, jitter_ms, packet_loss_pct,
        resolution, fps, audio_level, connection_type, turn_server, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.roomId,
        request.participantId ?? null,
        request.metricType,
        request.bitrateKbps ?? null,
        request.latencyMs ?? null,
        request.jitterMs ?? null,
        request.packetLossPct ?? null,
        request.resolution ?? null,
        request.fps ?? null,
        request.audioLevel ?? null,
        request.connectionType ?? null,
        request.turnServer ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as LiveKitQualityMetricRecord;
  }

  async getRoomQualityMetrics(roomId: string): Promise<{
    avgBitrate: number | null;
    avgLatency: number | null;
    avgPacketLoss: number | null;
    participantCount: number;
  }> {
    const result = await this.query<{
      avg_bitrate: string | null;
      avg_latency: string | null;
      avg_packet_loss: string | null;
      participant_count: string;
    }>(
      `SELECT
        AVG(last_bitrate_kbps) as avg_bitrate,
        AVG(last_latency_ms) as avg_latency,
        AVG(last_packet_loss_pct) as avg_packet_loss,
        COUNT(*) as participant_count
      FROM nchat_livekit_participants
      WHERE source_account_id = $1 AND room_id = $2 AND status = 'joined'`,
      [this.sourceAccountId, roomId]
    );

    const row = result.rows[0];
    return {
      avgBitrate: row?.avg_bitrate ? parseFloat(row.avg_bitrate) : null,
      avgLatency: row?.avg_latency ? parseFloat(row.avg_latency) : null,
      avgPacketLoss: row?.avg_packet_loss ? parseFloat(row.avg_packet_loss) : null,
      participantCount: parseInt(row?.participant_count ?? '0', 10),
    };
  }

  async getParticipantQualityMetrics(roomId: string): Promise<Array<{
    userId: string;
    displayName: string | null;
    bitrate: number | null;
    latency: number | null;
    packetLoss: number | null;
    connectionType: string | null;
  }>> {
    const result = await this.query<{
      user_id: string;
      display_name: string | null;
      last_bitrate_kbps: number | null;
      last_latency_ms: number | null;
      last_packet_loss_pct: number | null;
      connection_type: string | null;
    }>(
      `SELECT p.user_id, p.display_name, p.last_bitrate_kbps, p.last_latency_ms, p.last_packet_loss_pct,
        (SELECT connection_type FROM nchat_livekit_quality_metrics
         WHERE source_account_id = $1 AND participant_id = p.id
         ORDER BY recorded_at DESC LIMIT 1) as connection_type
      FROM nchat_livekit_participants p
      WHERE p.source_account_id = $1 AND p.room_id = $2 AND p.status = 'joined'`,
      [this.sourceAccountId, roomId]
    );

    return result.rows.map(row => ({
      userId: row.user_id,
      displayName: row.display_name,
      bitrate: row.last_bitrate_kbps,
      latency: row.last_latency_ms,
      packetLoss: row.last_packet_loss_pct,
      connectionType: row.connection_type,
    }));
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<LiveKitStats> {
    const roomsResult = await this.query<{ total: string; active: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE status = 'active') as active
      FROM nchat_livekit_rooms
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const participantsResult = await this.query<{ total: string; active: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE status = 'joined') as active
      FROM nchat_livekit_participants
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const egressResult = await this.query<{ total: string; active: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE status = 'active') as active
      FROM nchat_livekit_egress_jobs
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const tokensResult = await this.query<{ total: string }>(
      'SELECT COUNT(*) as total FROM nchat_livekit_tokens WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    return {
      totalRooms: parseInt(roomsResult.rows[0]?.total ?? '0', 10),
      activeRooms: parseInt(roomsResult.rows[0]?.active ?? '0', 10),
      totalParticipants: parseInt(participantsResult.rows[0]?.total ?? '0', 10),
      activeParticipants: parseInt(participantsResult.rows[0]?.active ?? '0', 10),
      totalEgressJobs: parseInt(egressResult.rows[0]?.total ?? '0', 10),
      activeEgressJobs: parseInt(egressResult.rows[0]?.active ?? '0', 10),
      totalTokensIssued: parseInt(tokensResult.rows[0]?.total ?? '0', 10),
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO nchat_livekit_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)`,
      [
        `evt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
        this.sourceAccountId,
        eventType,
        JSON.stringify(payload),
      ]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE nchat_livekit_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [eventId, error ?? null]
    );
  }
}
