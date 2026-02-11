/**
 * Geocoding Plugin Types
 * Complete type definitions for geocoding, geofences, and places
 */

// =============================================================================
// Enums and Literals
// =============================================================================

export type QueryType = 'forward' | 'reverse' | 'autocomplete' | 'place_search';

export type FenceType = 'circle' | 'polygon';

export type GeofenceEventType = 'enter' | 'exit';

export type Accuracy = 'rooftop' | 'range_interpolated' | 'geometric_center' | 'approximate';

// =============================================================================
// Database Record Types
// =============================================================================

export interface GeoCacheRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  query_type: QueryType;
  query_hash: string;
  query_text: string;
  provider: string;
  lat: number | null;
  lng: number | null;
  formatted_address: string | null;
  street_number: string | null;
  street_name: string | null;
  city: string | null;
  state: string | null;
  state_code: string | null;
  country: string | null;
  country_code: string | null;
  postal_code: string | null;
  place_id: string | null;
  place_type: string | null;
  accuracy: Accuracy | null;
  bounds: Record<string, unknown> | null;
  raw_response: Record<string, unknown> | null;
  hit_count: number;
  created_at: Date;
  updated_at: Date;
  expires_at: Date | null;
}

export interface GeofenceRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  fence_type: FenceType;
  center_lat: number;
  center_lng: number;
  radius_meters: number | null;
  polygon: Record<string, unknown> | null;
  active: boolean;
  notify_on_enter: boolean;
  notify_on_exit: boolean;
  notify_url: string | null;
  metadata: Record<string, unknown>;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
  deleted_at: Date | null;
}

export interface GeofenceEventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  geofence_id: string;
  event_type: GeofenceEventType;
  entity_id: string;
  entity_type: string;
  lat: number;
  lng: number;
  notified: boolean;
  notified_at: Date | null;
  created_at: Date;
}

export interface PlaceRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  provider: string;
  provider_place_id: string;
  name: string;
  category: string | null;
  lat: number;
  lng: number;
  formatted_address: string | null;
  phone: string | null;
  website: string | null;
  rating: number | null;
  review_count: number | null;
  hours: Record<string, unknown> | null;
  photos: unknown[];
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Request Types
// =============================================================================

export interface ForwardGeocodeRequest {
  address: string;
  city?: string;
  state?: string;
  country?: string;
  limit?: number;
}

export interface ReverseGeocodeRequest {
  lat: number;
  lng: number;
  limit?: number;
}

export interface PlaceSearchRequest {
  query: string;
  lat?: number;
  lng?: number;
  radius?: number;
  category?: string;
  limit?: number;
}

export interface AutocompleteRequest {
  input: string;
  lat?: number;
  lng?: number;
  types?: string[];
}

export interface BatchGeocodeRequest {
  addresses: string[];
}

export interface CreateGeofenceRequest {
  name: string;
  description?: string;
  fence_type?: FenceType;
  center_lat: number;
  center_lng: number;
  radius_meters?: number;
  polygon?: Record<string, unknown>;
  notify_on_enter?: boolean;
  notify_on_exit?: boolean;
  notify_url?: string;
  metadata?: Record<string, unknown>;
  created_by?: string;
}

export interface UpdateGeofenceRequest {
  name?: string;
  description?: string;
  center_lat?: number;
  center_lng?: number;
  radius_meters?: number;
  polygon?: Record<string, unknown>;
  active?: boolean;
  notify_on_enter?: boolean;
  notify_on_exit?: boolean;
  notify_url?: string;
  metadata?: Record<string, unknown>;
}

export interface EvaluateGeofenceRequest {
  lat: number;
  lng: number;
  entity_id?: string;
  entity_type?: string;
}

export interface ClearCacheRequest {
  older_than_days?: number;
}

// =============================================================================
// Response Types
// =============================================================================

export interface GeoResult {
  lat: number;
  lng: number;
  formatted_address: string | null;
  street_number: string | null;
  street_name: string | null;
  city: string | null;
  state: string | null;
  state_code: string | null;
  country: string | null;
  country_code: string | null;
  postal_code: string | null;
  place_id: string | null;
  place_type: string | null;
  accuracy: Accuracy | null;
  provider: string;
  cached: boolean;
  stale?: boolean;
}

export interface AutocompleteResult {
  description: string;
  place_id: string;
  structured: {
    main_text: string;
    secondary_text: string;
  };
}

export interface GeofenceEvaluation {
  inside: boolean;
  geofence: GeofenceRecord;
  distance_meters: number;
}

export interface CacheStatsResponse {
  total_entries: number;
  active_entries: number;
  expired_entries: number;
  total_hits: number;
  avg_hits_per_entry: number;
  reuse_percentage: number;
  by_query_type: Record<string, number>;
  by_provider: Record<string, number>;
}

export interface PluginStats {
  total_cache_entries: number;
  total_geofences: number;
  active_geofences: number;
  total_geofence_events: number;
  total_places: number;
  cache_hit_rate: number;
  by_provider: Record<string, number>;
}
