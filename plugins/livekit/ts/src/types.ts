/**
 * LiveKit Plugin Types
 * Complete type definitions for LiveKit voice/video infrastructure
 */

// =============================================================================
// Room Types
// =============================================================================

export type RoomType = 'call' | 'stream' | 'webinar' | 'broadcast';
export type RoomStatus = 'creating' | 'active' | 'closing' | 'closed' | 'failed';

export interface LiveKitRoomRecord {
  id: string;
  source_account_id: string;
  livekit_room_name: string;
  livekit_room_sid: string | null;
  room_type: RoomType;
  max_participants: number;
  empty_timeout: number;
  call_id: string | null;
  stream_id: string | null;
  status: RoomStatus;
  created_at: Date;
  activated_at: Date | null;
  closed_at: Date | null;
  metadata: Record<string, unknown>;
  updated_at: Date;
}

export interface CreateRoomRequest {
  roomName: string;
  roomType: RoomType;
  maxParticipants?: number;
  emptyTimeout?: number;
  callId?: string;
  streamId?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateRoomRequest {
  livekitRoomSid?: string;
  status?: RoomStatus;
  maxParticipants?: number;
  emptyTimeout?: number;
  activatedAt?: string;
  closedAt?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Participant Types
// =============================================================================

export type ParticipantStatus = 'joining' | 'joined' | 'reconnecting' | 'disconnected';

export interface LiveKitParticipantRecord {
  id: string;
  source_account_id: string;
  room_id: string;
  user_id: string;
  livekit_participant_sid: string | null;
  livekit_identity: string;
  display_name: string | null;
  metadata: Record<string, unknown>;
  status: ParticipantStatus;
  camera_enabled: boolean;
  microphone_enabled: boolean;
  screen_share_enabled: boolean;
  last_bitrate_kbps: number | null;
  last_latency_ms: number | null;
  last_packet_loss_pct: number | null;
  joined_at: Date;
  left_at: Date | null;
  total_duration_seconds: number;
  created_at: Date;
  updated_at: Date;
}

export interface CreateParticipantRequest {
  roomId: string;
  userId: string;
  livekitIdentity: string;
  displayName?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateParticipantRequest {
  livekitParticipantSid?: string;
  status?: ParticipantStatus;
  displayName?: string;
  cameraEnabled?: boolean;
  microphoneEnabled?: boolean;
  screenShareEnabled?: boolean;
  lastBitrateKbps?: number;
  lastLatencyMs?: number;
  lastPacketLossPct?: number;
  leftAt?: string;
  totalDurationSeconds?: number;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Egress Job Types
// =============================================================================

export type EgressType = 'track' | 'room' | 'participant' | 'stream';
export type EgressOutputType = 'file' | 'stream' | 'segments' | 'images';
export type EgressStatus = 'pending' | 'active' | 'ending' | 'complete' | 'failed';

export interface LiveKitEgressJobRecord {
  id: string;
  source_account_id: string;
  room_id: string;
  recording_id: string | null;
  livekit_egress_id: string;
  egress_type: EgressType;
  output_type: EgressOutputType;
  config: Record<string, unknown>;
  status: EgressStatus;
  file_url: string | null;
  file_size_bytes: number | null;
  duration_seconds: number | null;
  playlist_url: string | null;
  error_message: string | null;
  started_at: Date;
  ended_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateEgressJobRequest {
  roomId: string;
  livekitEgressId: string;
  egressType: EgressType;
  outputType: EgressOutputType;
  config?: Record<string, unknown>;
  recordingId?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateEgressJobRequest {
  status?: EgressStatus;
  fileUrl?: string;
  fileSizeBytes?: number;
  durationSeconds?: number;
  playlistUrl?: string;
  errorMessage?: string;
  endedAt?: string;
  metadata?: Record<string, unknown>;
}

export interface StartRoomCompositeRequest {
  roomName: string;
  layout?: string;
  audioOnly?: boolean;
  videoOptions?: {
    width?: number;
    height?: number;
    depth?: number;
    framerate?: number;
  };
  fileOutput?: {
    fileType?: string;
    filepath?: string;
  };
}

export interface StartTrackEgressRequest {
  roomName: string;
  trackSid: string;
  fileOutput?: {
    fileType?: string;
    filepath?: string;
  };
}

export interface StartStreamEgressRequest {
  roomName: string;
  urls: string[];
  streamProtocol?: string;
}

// =============================================================================
// Token Types
// =============================================================================

export interface LiveKitTokenRecord {
  id: string;
  source_account_id: string;
  room_id: string;
  user_id: string;
  token_hash: string;
  grants: Record<string, unknown>;
  issued_at: Date;
  expires_at: Date;
  revoked_at: Date | null;
  revoked_by: string | null;
  revoke_reason: string | null;
  first_used_at: Date | null;
  last_used_at: Date | null;
  use_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface CreateTokenRequest {
  roomName: string;
  participantIdentity: string;
  participantName?: string;
  grants?: {
    canPublish?: boolean;
    canSubscribe?: boolean;
    canPublishData?: boolean;
    canPublishSources?: string[];
  };
  ttl?: number;
}

export interface TokenResponse {
  success: boolean;
  token: string;
  tokenId: string;
  livekitUrl: string;
  expiresAt: string;
}

// =============================================================================
// Quality Metrics Types
// =============================================================================

export type MetricType = 'connection' | 'audio' | 'video' | 'screen_share';
export type ConnectionType = 'relay' | 'srflx' | 'host' | 'prflx';

export interface LiveKitQualityMetricRecord {
  id: string;
  source_account_id: string;
  room_id: string;
  participant_id: string | null;
  metric_type: MetricType;
  bitrate_kbps: number | null;
  latency_ms: number | null;
  jitter_ms: number | null;
  packet_loss_pct: number | null;
  resolution: string | null;
  fps: number | null;
  audio_level: number | null;
  connection_type: ConnectionType | null;
  turn_server: string | null;
  metadata: Record<string, unknown>;
  recorded_at: Date;
}

export interface CreateQualityMetricRequest {
  roomId: string;
  participantId?: string;
  metricType: MetricType;
  bitrateKbps?: number;
  latencyMs?: number;
  jitterMs?: number;
  packetLossPct?: number;
  resolution?: string;
  fps?: number;
  audioLevel?: number;
  connectionType?: ConnectionType;
  turnServer?: string;
  metadata?: Record<string, unknown>;
}

export interface RoomQualityResponse {
  success: boolean;
  room: {
    avgBitrate: number | null;
    avgLatency: number | null;
    avgPacketLoss: number | null;
    participantCount: number;
  };
  participants: Array<{
    userId: string;
    displayName: string | null;
    bitrate: number | null;
    latency: number | null;
    packetLoss: number | null;
    connectionType: string | null;
  }>;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface LiveKitWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface LiveKitStats {
  totalRooms: number;
  activeRooms: number;
  totalParticipants: number;
  activeParticipants: number;
  totalEgressJobs: number;
  activeEgressJobs: number;
  totalTokensIssued: number;
}
