/**
 * LiveKit Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // LiveKit Server
  livekitUrl: string;
  livekitApiKey: string;
  livekitApiSecret: string;

  // LiveKit Server Ports
  livekitPort: number;
  livekitRtmpPort: number;
  livekitTurnPort: number;

  // Recording/Egress
  egressEnabled: boolean;
  recordingsPath: string;
  recordingsS3Bucket: string;

  // Quality Monitoring
  qualityMonitoringEnabled: boolean;
  qualitySampleInterval: number;

  // Token Defaults
  tokenDefaultTtl: number;
  tokenMaxTtl: number;

  // Room Defaults
  roomDefaultMaxParticipants: number;
  roomEmptyTimeout: number;

  // Resource Limits
  maxConcurrentRooms: number;
  maxParticipantsPerRoom: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('LIVEKIT');

  const config: Config = {
    // Server
    port: parseInt(process.env.LIVEKIT_PLUGIN_PORT ?? process.env.PORT ?? '3707', 10),
    host: process.env.LIVEKIT_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // LiveKit Server
    livekitUrl: process.env.LIVEKIT_URL ?? 'wss://localhost:7880',
    livekitApiKey: process.env.LIVEKIT_API_KEY ?? '',
    livekitApiSecret: process.env.LIVEKIT_API_SECRET ?? '',

    // LiveKit Server Ports
    livekitPort: parseInt(process.env.LIVEKIT_PORT ?? '7880', 10),
    livekitRtmpPort: parseInt(process.env.LIVEKIT_RTMP_PORT ?? '1935', 10),
    livekitTurnPort: parseInt(process.env.LIVEKIT_TURN_PORT ?? '3478', 10),

    // Recording/Egress
    egressEnabled: process.env.LIVEKIT_EGRESS_ENABLED !== 'false',
    recordingsPath: process.env.LIVEKIT_RECORDINGS_PATH ?? '/var/livekit/recordings',
    recordingsS3Bucket: process.env.LIVEKIT_RECORDINGS_S3_BUCKET ?? '',

    // Quality Monitoring
    qualityMonitoringEnabled: process.env.LIVEKIT_QUALITY_MONITORING_ENABLED !== 'false',
    qualitySampleInterval: parseInt(process.env.LIVEKIT_QUALITY_SAMPLE_INTERVAL ?? '10', 10),

    // Token Defaults
    tokenDefaultTtl: parseInt(process.env.LIVEKIT_TOKEN_DEFAULT_TTL ?? '3600', 10),
    tokenMaxTtl: parseInt(process.env.LIVEKIT_TOKEN_MAX_TTL ?? '86400', 10),

    // Room Defaults
    roomDefaultMaxParticipants: parseInt(process.env.LIVEKIT_ROOM_DEFAULT_MAX_PARTICIPANTS ?? '100', 10),
    roomEmptyTimeout: parseInt(process.env.LIVEKIT_ROOM_EMPTY_TIMEOUT ?? '300', 10),

    // Resource Limits
    maxConcurrentRooms: parseInt(process.env.LIVEKIT_MAX_CONCURRENT_ROOMS ?? '100', 10),
    maxParticipantsPerRoom: parseInt(process.env.LIVEKIT_MAX_PARTICIPANTS_PER_ROOM ?? '200', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? process.env.LIVEKIT_LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.tokenDefaultTtl > config.tokenMaxTtl) {
    throw new Error('LIVEKIT_TOKEN_DEFAULT_TTL cannot exceed LIVEKIT_TOKEN_MAX_TTL');
  }

  if (config.roomDefaultMaxParticipants > config.maxParticipantsPerRoom) {
    throw new Error('LIVEKIT_ROOM_DEFAULT_MAX_PARTICIPANTS cannot exceed LIVEKIT_MAX_PARTICIPANTS_PER_ROOM');
  }

  return config;
}
