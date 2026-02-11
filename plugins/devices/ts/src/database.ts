/**
 * Devices Database Operations
 * Complete CRUD operations for devices, commands, telemetry, ingest sessions, and audit logs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  DeviceRecord,
  CommandRecord,
  TelemetryRecord,
  IngestSessionRecord,
  AuditLogRecord,
  DeviceStatus,
  TrustLevel,
  DeviceType,
  CommandType,
  CommandStatus,
  TelemetryType,
  IngestStatus,
  RegisterDeviceRequest,
  UpdateDeviceRequest,
  DispatchCommandRequest,
  SubmitTelemetryRequest,
  StartIngestRequest,
  IngestHeartbeatRequest,
  FleetStats,
  DeviceHealth,
} from './types.js';

const logger = createLogger('devices:db');

export class DevicesDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(sourceAccountId: string): DevicesDatabase {
    return new DevicesDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing devices schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Devices
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS dev_devices (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        device_id VARCHAR(255) NOT NULL,
        name VARCHAR(255),
        device_type VARCHAR(64) NOT NULL,
        model VARCHAR(128),
        firmware_version VARCHAR(64),
        status VARCHAR(32) NOT NULL DEFAULT 'unregistered',
        trust_level VARCHAR(16) NOT NULL DEFAULT 'untrusted',
        enrollment_token VARCHAR(255),
        enrollment_challenge VARCHAR(255),
        enrolled_at TIMESTAMPTZ,
        enrolled_by VARCHAR(255),
        public_key TEXT,
        last_seen_at TIMESTAMPTZ,
        last_ip VARCHAR(45),
        capabilities JSONB DEFAULT '[]',
        config JSONB DEFAULT '{}',
        labels JSONB DEFAULT '{}',
        metadata JSONB DEFAULT '{}',
        revoked_at TIMESTAMPTZ,
        revoked_by VARCHAR(255),
        revoke_reason TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, app_id, device_id)
      );

      CREATE INDEX IF NOT EXISTS idx_dev_devices_source_account
        ON dev_devices(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dev_devices_app
        ON dev_devices(app_id);
      CREATE INDEX IF NOT EXISTS idx_dev_devices_status
        ON dev_devices(status);
      CREATE INDEX IF NOT EXISTS idx_dev_devices_type
        ON dev_devices(device_type);
      CREATE INDEX IF NOT EXISTS idx_dev_devices_trust
        ON dev_devices(trust_level);
      CREATE INDEX IF NOT EXISTS idx_dev_devices_last_seen
        ON dev_devices(last_seen_at);

      -- =====================================================================
      -- Commands
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS dev_commands (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        device_id UUID NOT NULL REFERENCES dev_devices(id),
        command_type VARCHAR(64) NOT NULL,
        command_id VARCHAR(255) NOT NULL,
        payload JSONB NOT NULL DEFAULT '{}',
        status VARCHAR(32) NOT NULL DEFAULT 'dispatched',
        priority VARCHAR(16) DEFAULT 'normal',
        dispatched_at TIMESTAMPTZ DEFAULT NOW(),
        acked_at TIMESTAMPTZ,
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        result JSONB,
        error TEXT,
        timeout_seconds INTEGER DEFAULT 300,
        deadline TIMESTAMPTZ,
        retry_count INTEGER DEFAULT 0,
        max_retries INTEGER DEFAULT 3,
        idempotency_key VARCHAR(255) NOT NULL,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, app_id, idempotency_key)
      );

      CREATE INDEX IF NOT EXISTS idx_dev_commands_source_account
        ON dev_commands(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dev_commands_device
        ON dev_commands(device_id);
      CREATE INDEX IF NOT EXISTS idx_dev_commands_status
        ON dev_commands(status);
      CREATE INDEX IF NOT EXISTS idx_dev_commands_deadline
        ON dev_commands(deadline) WHERE status IN ('dispatched', 'acked', 'running');
      CREATE INDEX IF NOT EXISTS idx_dev_commands_type
        ON dev_commands(command_type);

      -- =====================================================================
      -- Telemetry
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS dev_telemetry (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        device_id UUID NOT NULL REFERENCES dev_devices(id),
        telemetry_type VARCHAR(64) NOT NULL,
        data JSONB NOT NULL,
        recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        received_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_dev_telemetry_source_account
        ON dev_telemetry(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dev_telemetry_device
        ON dev_telemetry(device_id);
      CREATE INDEX IF NOT EXISTS idx_dev_telemetry_type
        ON dev_telemetry(telemetry_type);
      CREATE INDEX IF NOT EXISTS idx_dev_telemetry_recorded
        ON dev_telemetry(recorded_at);

      -- =====================================================================
      -- Ingest Sessions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS dev_ingest_sessions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        device_id UUID NOT NULL REFERENCES dev_devices(id),
        stream_id VARCHAR(255) NOT NULL,
        status VARCHAR(32) NOT NULL DEFAULT 'idle',
        ingest_url TEXT,
        protocol VARCHAR(16) DEFAULT 'rtmp',
        channel VARCHAR(128),
        quality VARCHAR(16),
        bitrate_kbps INTEGER,
        fps FLOAT,
        resolution VARCHAR(16),
        started_at TIMESTAMPTZ,
        last_heartbeat_at TIMESTAMPTZ,
        ended_at TIMESTAMPTZ,
        bytes_ingested BIGINT DEFAULT 0,
        frames_dropped INTEGER DEFAULT 0,
        error_count INTEGER DEFAULT 0,
        last_error TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_dev_ingest_source_account
        ON dev_ingest_sessions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dev_ingest_device
        ON dev_ingest_sessions(device_id);
      CREATE INDEX IF NOT EXISTS idx_dev_ingest_status
        ON dev_ingest_sessions(status) WHERE status = 'active';
      CREATE INDEX IF NOT EXISTS idx_dev_ingest_stream
        ON dev_ingest_sessions(stream_id);

      -- =====================================================================
      -- Audit Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS dev_audit_log (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        device_id UUID REFERENCES dev_devices(id),
        action VARCHAR(64) NOT NULL,
        actor_id VARCHAR(255),
        details JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_dev_audit_source_account
        ON dev_audit_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dev_audit_device
        ON dev_audit_log(device_id);
      CREATE INDEX IF NOT EXISTS idx_dev_audit_action
        ON dev_audit_log(action);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW dev_devices_by_status AS
      SELECT source_account_id, app_id, status, trust_level, device_type,
             COUNT(*) AS device_count
      FROM dev_devices
      GROUP BY source_account_id, app_id, status, trust_level, device_type
      ORDER BY source_account_id, app_id, status, trust_level;

      CREATE OR REPLACE VIEW dev_command_success_rate AS
      SELECT d.source_account_id, d.app_id, d.device_id, c.command_type,
             COUNT(*) AS total_commands,
             COUNT(*) FILTER (WHERE c.status = 'succeeded') AS succeeded,
             COUNT(*) FILTER (WHERE c.status = 'failed') AS failed,
             COUNT(*) FILTER (WHERE c.status = 'timeout') AS timed_out,
             ROUND(100.0 * COUNT(*) FILTER (WHERE c.status = 'succeeded') / NULLIF(COUNT(*), 0), 2) AS success_rate
      FROM dev_commands c
      JOIN dev_devices d ON c.device_id = d.id
      GROUP BY d.source_account_id, d.app_id, d.device_id, c.command_type;

      CREATE OR REPLACE VIEW dev_ingest_uptime AS
      SELECT i.source_account_id, i.app_id, i.device_id, i.stream_id, i.status,
             i.started_at,
             EXTRACT(EPOCH FROM (COALESCE(i.ended_at, NOW()) - i.started_at)) / 3600 AS uptime_hours,
             i.bytes_ingested,
             i.frames_dropped,
             i.error_count
      FROM dev_ingest_sessions i
      WHERE i.status IN ('active', 'degraded')
      ORDER BY i.started_at DESC;

      CREATE OR REPLACE VIEW dev_health_trends AS
      SELECT t.source_account_id, t.device_id, t.telemetry_type,
             DATE_TRUNC('hour', t.recorded_at) AS hour,
             COUNT(*) AS data_points,
             AVG((t.data->>'value')::FLOAT) AS avg_value,
             MAX((t.data->>'value')::FLOAT) AS max_value,
             MIN((t.data->>'value')::FLOAT) AS min_value
      FROM dev_telemetry t
      WHERE t.recorded_at >= NOW() - INTERVAL '24 hours'
        AND t.data ? 'value'
      GROUP BY t.source_account_id, t.device_id, t.telemetry_type, DATE_TRUNC('hour', t.recorded_at)
      ORDER BY t.device_id, t.telemetry_type, hour;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Devices
  // =========================================================================

  async registerDevice(appId: string, request: RegisterDeviceRequest): Promise<DeviceRecord> {
    const result = await this.query<DeviceRecord>(
      `INSERT INTO dev_devices (
        source_account_id, app_id, device_id, name, device_type,
        model, firmware_version, status, capabilities, config, labels, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'unregistered', $8, $9, $10, $11)
      ON CONFLICT (source_account_id, app_id, device_id)
      DO UPDATE SET
        name = COALESCE(EXCLUDED.name, dev_devices.name),
        model = COALESCE(EXCLUDED.model, dev_devices.model),
        firmware_version = COALESCE(EXCLUDED.firmware_version, dev_devices.firmware_version),
        capabilities = COALESCE(EXCLUDED.capabilities, dev_devices.capabilities),
        config = COALESCE(EXCLUDED.config, dev_devices.config),
        labels = COALESCE(EXCLUDED.labels, dev_devices.labels),
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.device_id,
        request.name ?? null,
        request.device_type,
        request.model ?? null,
        request.firmware_version ?? null,
        JSON.stringify(request.capabilities ?? []),
        JSON.stringify(request.config ?? {}),
        JSON.stringify(request.labels ?? {}),
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getDevice(deviceId: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `SELECT * FROM dev_devices
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, deviceId]
    );
    return result.rows[0] ?? null;
  }

  async getDeviceByDeviceId(deviceId: string, appId?: string): Promise<DeviceRecord | null> {
    if (appId) {
      const result = await this.query<DeviceRecord>(
        `SELECT * FROM dev_devices
         WHERE source_account_id = $1 AND device_id = $2 AND app_id = $3`,
        [this.sourceAccountId, deviceId, appId]
      );
      return result.rows[0] ?? null;
    }

    const result = await this.query<DeviceRecord>(
      `SELECT * FROM dev_devices
       WHERE source_account_id = $1 AND device_id = $2`,
      [this.sourceAccountId, deviceId]
    );
    return result.rows[0] ?? null;
  }

  async listDevices(
    appId?: string,
    status?: DeviceStatus,
    deviceType?: DeviceType,
    trustLevel?: TrustLevel,
    limit = 100,
    offset = 0
  ): Promise<DeviceRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    if (deviceType) {
      conditions.push(`device_type = $${params.length + 1}`);
      params.push(deviceType);
    }

    if (trustLevel) {
      conditions.push(`trust_level = $${params.length + 1}`);
      params.push(trustLevel);
    }

    params.push(limit, offset);

    const result = await this.query<DeviceRecord>(
      `SELECT * FROM dev_devices
       WHERE ${conditions.join(' AND ')}
       ORDER BY updated_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateDevice(deviceId: string, updates: UpdateDeviceRequest): Promise<DeviceRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, deviceId];

    if (updates.name !== undefined) {
      setParts.push(`name = $${params.length + 1}`);
      params.push(updates.name);
    }

    if (updates.firmware_version !== undefined) {
      setParts.push(`firmware_version = $${params.length + 1}`);
      params.push(updates.firmware_version);
    }

    if (updates.capabilities !== undefined) {
      setParts.push(`capabilities = $${params.length + 1}`);
      params.push(JSON.stringify(updates.capabilities));
    }

    if (updates.config !== undefined) {
      setParts.push(`config = $${params.length + 1}`);
      params.push(JSON.stringify(updates.config));
    }

    if (updates.labels !== undefined) {
      setParts.push(`labels = $${params.length + 1}`);
      params.push(JSON.stringify(updates.labels));
    }

    if (updates.metadata !== undefined) {
      setParts.push(`metadata = $${params.length + 1}`);
      params.push(JSON.stringify(updates.metadata));
    }

    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async startEnrollment(deviceId: string, token: string, challenge: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET status = 'bootstrap_ready',
           trust_level = 'pending',
           enrollment_token = $3,
           enrollment_challenge = $4,
           updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, deviceId, token, challenge]
    );
    return result.rows[0] ?? null;
  }

  async completeEnrollment(deviceId: string, publicKey: string, enrolledBy?: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET status = 'enrolled',
           trust_level = 'trusted',
           public_key = $3,
           enrolled_at = NOW(),
           enrolled_by = $4,
           enrollment_token = NULL,
           enrollment_challenge = NULL,
           updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, deviceId, publicKey, enrolledBy ?? null]
    );
    return result.rows[0] ?? null;
  }

  async revokeDevice(deviceId: string, reason: string, revokedBy?: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET status = 'revoked',
           trust_level = 'untrusted',
           revoked_at = NOW(),
           revoked_by = $3,
           revoke_reason = $4,
           updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, deviceId, revokedBy ?? null, reason]
    );
    return result.rows[0] ?? null;
  }

  async suspendDevice(deviceId: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET status = 'suspended', updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND status = 'enrolled'
       RETURNING *`,
      [this.sourceAccountId, deviceId]
    );
    return result.rows[0] ?? null;
  }

  async reinstateDevice(deviceId: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET status = 'enrolled', updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND status = 'suspended'
       RETURNING *`,
      [this.sourceAccountId, deviceId]
    );
    return result.rows[0] ?? null;
  }

  async updateDeviceHeartbeat(deviceId: string, ip?: string): Promise<DeviceRecord | null> {
    const result = await this.query<DeviceRecord>(
      `UPDATE dev_devices
       SET last_seen_at = NOW(),
           last_ip = COALESCE($3, last_ip),
           updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, deviceId, ip ?? null]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Commands
  // =========================================================================

  async dispatchCommand(appId: string, deviceUuid: string, request: DispatchCommandRequest, timeoutSeconds: number): Promise<CommandRecord> {
    const commandId = `cmd-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const idempotencyKey = request.idempotency_key ?? commandId;
    const deadline = new Date(Date.now() + timeoutSeconds * 1000);

    const result = await this.query<CommandRecord>(
      `INSERT INTO dev_commands (
        source_account_id, app_id, device_id, command_type, command_id,
        payload, priority, timeout_seconds, deadline, max_retries,
        idempotency_key, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      ON CONFLICT (source_account_id, app_id, idempotency_key) DO NOTHING
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        deviceUuid,
        request.command_type,
        commandId,
        JSON.stringify(request.payload ?? {}),
        request.priority ?? 'normal',
        timeoutSeconds,
        deadline,
        request.idempotency_key ? 1 : 3, // Idempotent commands don't retry
        idempotencyKey,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    // If conflict (idempotent key already exists), return existing
    if (result.rows.length === 0) {
      const existing = await this.query<CommandRecord>(
        `SELECT * FROM dev_commands
         WHERE source_account_id = $1 AND app_id = $2 AND idempotency_key = $3`,
        [this.sourceAccountId, appId, idempotencyKey]
      );
      return existing.rows[0];
    }

    return result.rows[0];
  }

  async getCommand(commandId: string): Promise<CommandRecord | null> {
    const result = await this.query<CommandRecord>(
      `SELECT * FROM dev_commands
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, commandId]
    );
    return result.rows[0] ?? null;
  }

  async listCommands(
    deviceUuid?: string,
    status?: CommandStatus,
    commandType?: CommandType,
    limit = 100,
    offset = 0
  ): Promise<CommandRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (deviceUuid) {
      conditions.push(`device_id = $${params.length + 1}`);
      params.push(deviceUuid);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    if (commandType) {
      conditions.push(`command_type = $${params.length + 1}`);
      params.push(commandType);
    }

    params.push(limit, offset);

    const result = await this.query<CommandRecord>(
      `SELECT * FROM dev_commands
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateCommandStatus(
    commandId: string,
    status: CommandStatus,
    extra?: { result?: Record<string, unknown>; error?: string }
  ): Promise<CommandRecord | null> {
    const setParts: string[] = [`status = $3`];
    const params: unknown[] = [this.sourceAccountId, commandId, status];

    if (status === 'acked') {
      setParts.push('acked_at = NOW()');
    } else if (status === 'running') {
      setParts.push('started_at = COALESCE(started_at, NOW())');
    } else if (status === 'succeeded' || status === 'failed' || status === 'timeout') {
      setParts.push('completed_at = NOW()');
    }

    if (extra?.result !== undefined) {
      setParts.push(`result = $${params.length + 1}`);
      params.push(JSON.stringify(extra.result));
    }

    if (extra?.error !== undefined) {
      setParts.push(`error = $${params.length + 1}`);
      params.push(extra.error);
    }

    const result = await this.query<CommandRecord>(
      `UPDATE dev_commands
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async cancelCommand(commandId: string): Promise<CommandRecord | null> {
    const result = await this.query<CommandRecord>(
      `UPDATE dev_commands
       SET status = 'cancelled', completed_at = NOW()
       WHERE source_account_id = $1 AND id = $2
         AND status IN ('dispatched', 'acked')
       RETURNING *`,
      [this.sourceAccountId, commandId]
    );
    return result.rows[0] ?? null;
  }

  async getDeviceCommands(deviceUuid: string, limit = 50): Promise<CommandRecord[]> {
    return this.listCommands(deviceUuid, undefined, undefined, limit);
  }

  // =========================================================================
  // Telemetry
  // =========================================================================

  async submitTelemetry(appId: string, deviceUuid: string, request: SubmitTelemetryRequest): Promise<TelemetryRecord> {
    const result = await this.query<TelemetryRecord>(
      `INSERT INTO dev_telemetry (
        source_account_id, app_id, device_id, telemetry_type, data, recorded_at
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        deviceUuid,
        request.telemetry_type,
        JSON.stringify(request.data),
        request.recorded_at ?? new Date().toISOString(),
      ]
    );

    return result.rows[0];
  }

  async getDeviceTelemetry(
    deviceUuid: string,
    telemetryType?: TelemetryType,
    limit = 100,
    offset = 0
  ): Promise<TelemetryRecord[]> {
    const conditions = ['source_account_id = $1', 'device_id = $2'];
    const params: unknown[] = [this.sourceAccountId, deviceUuid];

    if (telemetryType) {
      conditions.push(`telemetry_type = $${params.length + 1}`);
      params.push(telemetryType);
    }

    params.push(limit, offset);

    const result = await this.query<TelemetryRecord>(
      `SELECT * FROM dev_telemetry
       WHERE ${conditions.join(' AND ')}
       ORDER BY recorded_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async cleanupOldTelemetry(retentionDays: number): Promise<number> {
    const rowCount = await this.execute(
      `DELETE FROM dev_telemetry
       WHERE source_account_id = $1
         AND recorded_at < NOW() - ($2 || ' days')::INTERVAL`,
      [this.sourceAccountId, retentionDays.toString()]
    );
    return rowCount;
  }

  // =========================================================================
  // Ingest Sessions
  // =========================================================================

  async startIngestSession(appId: string, deviceUuid: string, request: StartIngestRequest): Promise<IngestSessionRecord> {
    const result = await this.query<IngestSessionRecord>(
      `INSERT INTO dev_ingest_sessions (
        source_account_id, app_id, device_id, stream_id,
        status, protocol, channel, quality, metadata, started_at
      ) VALUES ($1, $2, $3, $4, 'connecting', $5, $6, $7, $8, NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        deviceUuid,
        request.stream_id,
        request.protocol ?? 'rtmp',
        request.channel ?? null,
        request.quality ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getIngestSession(sessionId: string): Promise<IngestSessionRecord | null> {
    const result = await this.query<IngestSessionRecord>(
      `SELECT * FROM dev_ingest_sessions
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, sessionId]
    );
    return result.rows[0] ?? null;
  }

  async listIngestSessions(
    deviceUuid?: string,
    status?: IngestStatus,
    limit = 100,
    offset = 0
  ): Promise<IngestSessionRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (deviceUuid) {
      conditions.push(`device_id = $${params.length + 1}`);
      params.push(deviceUuid);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.query<IngestSessionRecord>(
      `SELECT * FROM dev_ingest_sessions
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async getActiveIngestSessions(): Promise<IngestSessionRecord[]> {
    const result = await this.query<IngestSessionRecord>(
      `SELECT * FROM dev_ingest_sessions
       WHERE source_account_id = $1 AND status IN ('active', 'degraded', 'connecting')
       ORDER BY started_at DESC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async heartbeatIngestSession(sessionId: string, data: IngestHeartbeatRequest): Promise<IngestSessionRecord | null> {
    const setParts: string[] = ['last_heartbeat_at = NOW()', 'status = \'active\'', 'updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, sessionId];

    if (data.bytes_ingested !== undefined) {
      setParts.push(`bytes_ingested = bytes_ingested + $${params.length + 1}`);
      params.push(data.bytes_ingested);
    }

    if (data.frames_dropped !== undefined) {
      setParts.push(`frames_dropped = frames_dropped + $${params.length + 1}`);
      params.push(data.frames_dropped);
    }

    if (data.bitrate_kbps !== undefined) {
      setParts.push(`bitrate_kbps = $${params.length + 1}`);
      params.push(data.bitrate_kbps);
    }

    if (data.fps !== undefined) {
      setParts.push(`fps = $${params.length + 1}`);
      params.push(data.fps);
    }

    if (data.resolution !== undefined) {
      setParts.push(`resolution = $${params.length + 1}`);
      params.push(data.resolution);
    }

    if (data.error_count !== undefined) {
      setParts.push(`error_count = $${params.length + 1}`);
      params.push(data.error_count);
    }

    if (data.last_error !== undefined) {
      setParts.push(`last_error = $${params.length + 1}`);
      params.push(data.last_error);
    }

    const result = await this.query<IngestSessionRecord>(
      `UPDATE dev_ingest_sessions
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async endIngestSession(sessionId: string): Promise<IngestSessionRecord | null> {
    const result = await this.query<IngestSessionRecord>(
      `UPDATE dev_ingest_sessions
       SET status = 'stopped', ended_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND status IN ('active', 'degraded', 'connecting', 'retrying')
       RETURNING *`,
      [this.sourceAccountId, sessionId]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Audit Log
  // =========================================================================

  async createAuditEntry(appId: string, action: string, deviceUuid?: string, actorId?: string, details?: Record<string, unknown>): Promise<AuditLogRecord> {
    const result = await this.query<AuditLogRecord>(
      `INSERT INTO dev_audit_log (
        source_account_id, app_id, device_id, action, actor_id, details
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        deviceUuid ?? null,
        action,
        actorId ?? null,
        JSON.stringify(details ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getDeviceAuditLog(deviceUuid: string, limit = 100, offset = 0): Promise<AuditLogRecord[]> {
    const result = await this.query<AuditLogRecord>(
      `SELECT * FROM dev_audit_log
       WHERE source_account_id = $1 AND device_id = $2
       ORDER BY created_at DESC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, deviceUuid, limit, offset]
    );
    return result.rows;
  }

  // =========================================================================
  // Statistics & Health
  // =========================================================================

  async getFleetStats(): Promise<FleetStats> {
    const result = await this.query<{
      total_devices: number;
      enrolled_devices: number;
      online_devices: number;
      suspended_devices: number;
      revoked_devices: number;
      last_activity: Date | null;
    }>(
      `SELECT
        COUNT(*) AS total_devices,
        COUNT(*) FILTER (WHERE status = 'enrolled') AS enrolled_devices,
        COUNT(*) FILTER (WHERE last_seen_at > NOW() - INTERVAL '5 minutes') AS online_devices,
        COUNT(*) FILTER (WHERE status = 'suspended') AS suspended_devices,
        COUNT(*) FILTER (WHERE status = 'revoked') AS revoked_devices,
        MAX(last_seen_at) AS last_activity
       FROM dev_devices
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const cmdResult = await this.query<{
      total_commands: number;
      pending_commands: number;
      succeeded_commands: number;
      failed_commands: number;
    }>(
      `SELECT
        COUNT(*) AS total_commands,
        COUNT(*) FILTER (WHERE status IN ('dispatched', 'acked', 'running')) AS pending_commands,
        COUNT(*) FILTER (WHERE status = 'succeeded') AS succeeded_commands,
        COUNT(*) FILTER (WHERE status = 'failed') AS failed_commands
       FROM dev_commands
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const ingestResult = await this.query<{ active_ingest_sessions: number }>(
      `SELECT COUNT(*) AS active_ingest_sessions
       FROM dev_ingest_sessions
       WHERE source_account_id = $1 AND status IN ('active', 'degraded')`,
      [this.sourceAccountId]
    );

    const telResult = await this.query<{ total_telemetry_records: number }>(
      `SELECT COUNT(*) AS total_telemetry_records
       FROM dev_telemetry
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    const crow = cmdResult.rows[0];
    const irow = ingestResult.rows[0];
    const trow = telResult.rows[0];

    return {
      total_devices: row?.total_devices ?? 0,
      enrolled_devices: row?.enrolled_devices ?? 0,
      online_devices: row?.online_devices ?? 0,
      suspended_devices: row?.suspended_devices ?? 0,
      revoked_devices: row?.revoked_devices ?? 0,
      total_commands: crow?.total_commands ?? 0,
      pending_commands: crow?.pending_commands ?? 0,
      succeeded_commands: crow?.succeeded_commands ?? 0,
      failed_commands: crow?.failed_commands ?? 0,
      active_ingest_sessions: irow?.active_ingest_sessions ?? 0,
      total_telemetry_records: trow?.total_telemetry_records ?? 0,
      last_activity: row?.last_activity ?? null,
    };
  }

  async getDeviceHealth(deviceUuid: string): Promise<DeviceHealth | null> {
    const device = await this.getDevice(deviceUuid);
    if (!device) return null;

    const telemetry = await this.getDeviceTelemetry(deviceUuid, undefined, 20);
    const pendingCmds = await this.listCommands(deviceUuid, 'dispatched');
    const activeSessions = await this.listIngestSessions(deviceUuid, 'active');

    return {
      device_id: device.device_id,
      name: device.name,
      status: device.status,
      trust_level: device.trust_level,
      last_seen_at: device.last_seen_at,
      recent_telemetry: telemetry,
      pending_commands: pendingCmds.length,
      active_ingest_sessions: activeSessions.length,
    };
  }

  async getDiagnostics(deviceUuid: string): Promise<Record<string, unknown>> {
    const device = await this.getDevice(deviceUuid);
    if (!device) return {};

    const recentTelemetry = await this.getDeviceTelemetry(deviceUuid, undefined, 50);
    const recentCommands = await this.listCommands(deviceUuid, undefined, undefined, 20);
    const ingestSessions = await this.listIngestSessions(deviceUuid, undefined, 10);
    const auditLog = await this.getDeviceAuditLog(deviceUuid, 20);

    return {
      device,
      recent_telemetry: recentTelemetry,
      recent_commands: recentCommands,
      ingest_sessions: ingestSessions,
      audit_log: auditLog,
    };
  }
}
