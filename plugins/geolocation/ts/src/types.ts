/**
 * Geolocation Plugin Types
 * All TypeScript interfaces for the geolocation plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface GeoLocationRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  device_id: string | null;
  latitude: number;
  longitude: number;
  altitude: number | null;
  accuracy: number | null;
  speed: number | null;
  heading: number | null;
  battery_level: number | null;
  is_charging: boolean | null;
  activity_type: string | null;
  address: string | null;
  metadata: Record<string, unknown>;
  recorded_at: Date;
  created_at: Date;
}

export interface GeoLatestRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  device_id: string | null;
  latitude: number;
  longitude: number;
  altitude: number | null;
  accuracy: number | null;
  speed: number | null;
  heading: number | null;
  battery_level: number | null;
  is_charging: boolean | null;
  activity_type: string | null;
  address: string | null;
  recorded_at: Date;
}

export interface GeoFenceRecord {
  id: string;
  source_account_id: string;
  owner_id: string;
  name: string;
  description: string | null;
  fence_type: string;
  latitude: number;
  longitude: number;
  radius_meters: number | null;
  polygon: Record<string, unknown> | null;
  address: string | null;
  trigger_on: string;
  active: boolean;
  schedule: Record<string, unknown> | null;
  notify_user_ids: string[];
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface GeoFenceEventRecord {
  id: string;
  source_account_id: string;
  fence_id: string;
  user_id: string;
  event_type: string;
  latitude: number;
  longitude: number;
  triggered_at: Date;
}

export interface GeoWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface UpdateLocationRequest {
  userId: string;
  deviceId?: string;
  latitude: number;
  longitude: number;
  altitude?: number;
  accuracy?: number;
  speed?: number;
  heading?: number;
  batteryLevel?: number;
  isCharging?: boolean;
  activityType?: string;
  address?: string;
  recordedAt?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateLocationResponse {
  stored: true;
  geofenceEvents?: Array<{
    fenceId: string;
    fenceName: string;
    eventType: 'enter' | 'exit';
  }>;
}

export interface BatchLocationRequest {
  userId: string;
  deviceId?: string;
  locations: Array<{
    latitude: number;
    longitude: number;
    altitude?: number;
    accuracy?: number;
    speed?: number;
    heading?: number;
    batteryLevel?: number;
    isCharging?: boolean;
    activityType?: string;
    address?: string;
    recordedAt?: string;
    metadata?: Record<string, unknown>;
  }>;
}

export interface BatchLocationResponse {
  stored: number;
  total: number;
}

export interface GetLatestQuery {
  userIds?: string;
}

export interface GetHistoryQuery {
  userId: string;
  from?: string;
  to?: string;
  limit?: string;
  offset?: string;
}

export interface HistoryResponse {
  points: GeoLocationRecord[];
  total: number;
}

export interface DeleteHistoryRequest {
  userId: string;
  olderThan?: string;
}

export interface CreateGeofenceRequest {
  ownerId: string;
  name: string;
  description?: string;
  fenceType?: string;
  latitude: number;
  longitude: number;
  radiusMeters?: number;
  polygon?: Record<string, unknown>;
  address?: string;
  triggerOn?: string;
  active?: boolean;
  schedule?: Record<string, unknown>;
  notifyUserIds?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateGeofenceRequest {
  name?: string;
  description?: string;
  latitude?: number;
  longitude?: number;
  radiusMeters?: number;
  polygon?: Record<string, unknown>;
  address?: string;
  triggerOn?: string;
  active?: boolean;
  schedule?: Record<string, unknown>;
  notifyUserIds?: string[];
  metadata?: Record<string, unknown>;
}

export interface NearbyQuery {
  latitude: string;
  longitude: string;
  radiusMeters: string;
  userIds?: string;
}

export interface NearbyUser {
  userId: string;
  latitude: number;
  longitude: number;
  distanceMeters: number;
  lastSeenAt: string;
}

export interface DistanceQuery {
  userId1: string;
  userId2: string;
}

export interface DistanceResponse {
  distanceMeters: number;
  user1Location: { latitude: number; longitude: number; recordedAt: string };
  user2Location: { latitude: number; longitude: number; recordedAt: string };
}

export interface FenceEventsQuery {
  from?: string;
  to?: string;
  limit?: string;
}

export interface UserFenceEventsQuery {
  userId: string;
  from?: string;
  to?: string;
  limit?: string;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface GeolocationConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // PostGIS
  postgisEnabled: boolean;

  // History
  historyRetentionDays: number;

  // Batch
  batchMaxPoints: number;

  // Rate limiting
  minUpdateIntervalSeconds: number;

  // Geofence
  geofenceCheckOnUpdate: boolean;

  // Reverse geocoding
  reverseGeocodeEnabled: boolean;
  reverseGeocodeProvider: string;
  reverseGeocodeApiKey: string;

  // Battery
  lowBatteryThreshold: number;
}

// ============================================================================
// Stats Types
// ============================================================================

export interface GeolocationStats {
  totalLocations: number;
  totalUsers: number;
  totalFences: number;
  totalFenceEvents: number;
  lastLocationAt: string | null;
}

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}
