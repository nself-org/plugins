/**
 * Type definitions for realtime plugin
 */

// =============================================================================
// Configuration
// =============================================================================

export interface RealtimeConfig {
  port: number;
  host: string;
  redisUrl: string;
  corsOrigin: string[];
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;
  maxConnections: number;
  pingTimeout: number;
  pingInterval: number;
  jwtSecret?: string;
  allowAnonymous: boolean;
  enablePresence: boolean;
  enableTyping: boolean;
  typingTimeout: number;
  presenceHeartbeat: number;
  enableCompression: boolean;
  batchSize: number;
  rateLimit: number;
  logEvents: boolean;
  logEventTypes: string[];
  enableMetrics: boolean;
  metricsPath: string;
  enableHealth: boolean;
  healthPath: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';
}

// =============================================================================
// Database Models
// =============================================================================

export interface Connection {
  id: string;
  source_account_id: string;
  socket_id: string;
  user_id: string | null;
  session_id: string | null;
  status: 'connected' | 'disconnected' | 'reconnecting';
  transport: 'websocket' | 'polling';
  ip_address: string | null;
  user_agent: string | null;
  device_info: DeviceInfo | null;
  connected_at: Date;
  disconnected_at: Date | null;
  last_ping: Date;
  last_pong: Date;
  latency_ms: number | null;
  metadata: Record<string, unknown>;
}

export interface Room {
  id: string;
  source_account_id: string;
  name: string;
  type: 'channel' | 'dm' | 'group' | 'broadcast';
  visibility: 'public' | 'private' | 'secret';
  max_members: number | null;
  is_active: boolean;
  settings: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface RoomMember {
  id: string;
  source_account_id: string;
  room_id: string;
  user_id: string;
  role: 'admin' | 'moderator' | 'member' | 'guest';
  is_muted: boolean;
  is_banned: boolean;
  joined_at: Date;
  last_seen: Date;
  metadata: Record<string, unknown>;
}

export interface Presence {
  id: string;
  source_account_id: string;
  user_id: string;
  status: 'online' | 'away' | 'busy' | 'offline';
  custom_status: string | null;
  custom_emoji: string | null;
  last_active: Date;
  last_heartbeat: Date;
  expires_at: Date | null;
  connections_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface TypingIndicator {
  id: string;
  source_account_id: string;
  room_id: string;
  user_id: string;
  thread_id: string | null;
  started_at: Date;
  expires_at: Date;
}

export interface RealtimeEvent {
  id: string;
  source_account_id: string;
  event_type: string;
  socket_id: string | null;
  user_id: string | null;
  room_id: string | null;
  payload: Record<string, unknown>;
  ip_address: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Socket Events
// =============================================================================

export interface DeviceInfo {
  type: 'desktop' | 'mobile' | 'web';
  os?: string;
  browser?: string;
}

export interface AuthPayload {
  token: string;
  device?: DeviceInfo;
}

export interface JoinRoomPayload {
  roomName: string;
}

export interface LeaveRoomPayload {
  roomName: string;
}

export interface MessagePayload {
  roomName: string;
  content: string;
  threadId?: string;
  metadata?: Record<string, unknown>;
}

export interface TypingPayload {
  roomName: string;
  threadId?: string;
}

export interface PresencePayload {
  status: 'online' | 'away' | 'busy' | 'offline';
  customStatus?: {
    text: string;
    emoji?: string;
    expiresAt?: Date;
  };
}

export interface BroadcastPayload {
  event: string;
  data: unknown;
  roomName?: string;
}

// =============================================================================
// Server-to-Client Events
// =============================================================================

export interface ConnectedPayload {
  socketId: string;
  serverTime: Date;
  protocolVersion: string;
}

export interface AuthenticatedPayload {
  userId: string;
  sessionId: string;
  rooms: string[];
}

export interface RoomJoinedPayload {
  roomName: string;
  memberCount: number;
}

export interface RoomLeftPayload {
  roomName: string;
}

export interface UserJoinedPayload {
  roomName: string;
  userId: string;
}

export interface UserLeftPayload {
  roomName: string;
  userId: string;
}

export interface PresenceChangedPayload {
  userId: string;
  status: 'online' | 'away' | 'busy' | 'offline';
  customStatus?: string;
  customEmoji?: string;
}

export interface TypingEventPayload {
  roomName: string;
  threadId?: string;
  users: Array<{
    userId: string;
    startedAt: Date;
  }>;
}

export interface ErrorPayload {
  code: string;
  message: string;
  details?: unknown;
}

// =============================================================================
// Statistics
// =============================================================================

export interface ServerStats {
  uptime: number;
  connections: {
    total: number;
    active: number;
    authenticated: number;
    anonymous: number;
  };
  rooms: {
    total: number;
    active: number;
  };
  presence: {
    online: number;
    away: number;
    busy: number;
    offline: number;
  };
  events: {
    total: number;
    lastHour: number;
  };
  memory: {
    used: number;
    total: number;
    percentage: number;
  };
  cpu: {
    usage: number;
  };
}

// =============================================================================
// Response Types
// =============================================================================

export interface SocketResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: ErrorPayload;
}

export type SocketCallback<T = unknown> = (response: SocketResponse<T>) => void;
