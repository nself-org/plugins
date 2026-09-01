package internal

import (
	"encoding/json"
	"time"
)

// GeoCache represents a row in geo_cache.
type GeoCache struct {
	ID               string          `json:"id"`
	SourceAccountID  string          `json:"source_account_id"`
	QueryType        string          `json:"query_type"`
	QueryHash        string          `json:"query_hash"`
	QueryText        string          `json:"query_text"`
	Provider         string          `json:"provider"`
	Lat              *float64        `json:"lat"`
	Lng              *float64        `json:"lng"`
	FormattedAddress *string         `json:"formatted_address"`
	StreetNumber     *string         `json:"street_number"`
	StreetName       *string         `json:"street_name"`
	City             *string         `json:"city"`
	State            *string         `json:"state"`
	StateCode        *string         `json:"state_code"`
	Country          *string         `json:"country"`
	CountryCode      *string         `json:"country_code"`
	PostalCode       *string         `json:"postal_code"`
	PlaceID          *string         `json:"place_id"`
	PlaceType        *string         `json:"place_type"`
	Accuracy         *string         `json:"accuracy"`
	Bounds           json.RawMessage `json:"bounds"`
	RawResponse      json.RawMessage `json:"raw_response"`
	HitCount         int             `json:"hit_count"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	ExpiresAt        *time.Time      `json:"expires_at"`
}

// Geofence represents a row in geo_geofences.
type Geofence struct {
	ID               string          `json:"id"`
	SourceAccountID  string          `json:"source_account_id"`
	Name             string          `json:"name"`
	Description      *string         `json:"description"`
	FenceType        string          `json:"fence_type"`
	CenterLat        float64         `json:"center_lat"`
	CenterLng        float64         `json:"center_lng"`
	RadiusMeters     *float64        `json:"radius_meters"`
	Polygon          json.RawMessage `json:"polygon"`
	Active           bool            `json:"active"`
	NotifyOnEnter    bool            `json:"notify_on_enter"`
	NotifyOnExit     bool            `json:"notify_on_exit"`
	NotifyURL        *string         `json:"notify_url"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedBy        *string         `json:"created_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	DeletedAt        *time.Time      `json:"deleted_at,omitempty"`
}

// GeofenceEvent represents a row in geo_geofence_events.
type GeofenceEvent struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	GeofenceID       string     `json:"geofence_id"`
	EventType        string     `json:"event_type"`
	EntityID         string     `json:"entity_id"`
	EntityType       string     `json:"entity_type"`
	Lat              float64    `json:"lat"`
	Lng              float64    `json:"lng"`
	Notified         bool       `json:"notified"`
	NotifiedAt       *time.Time `json:"notified_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Place represents a row in geo_places.
type Place struct {
	ID               string          `json:"id"`
	SourceAccountID  string          `json:"source_account_id"`
	Provider         string          `json:"provider"`
	ProviderPlaceID  string          `json:"provider_place_id"`
	Name             string          `json:"name"`
	Category         *string         `json:"category"`
	Lat              float64         `json:"lat"`
	Lng              float64         `json:"lng"`
	FormattedAddress *string         `json:"formatted_address"`
	Phone            *string         `json:"phone"`
	Website          *string         `json:"website"`
	Rating           *float64        `json:"rating"`
	ReviewCount      *int            `json:"review_count"`
	Hours            json.RawMessage `json:"hours"`
	Photos           json.RawMessage `json:"photos"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// APIQuota represents a row in geo_api_quotas.
type APIQuota struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	QuotaType        string    `json:"quota_type"`
	PeriodStart      time.Time `json:"period_start"`
	APICalls         int       `json:"api_calls"`
	GeocodeCalls     int       `json:"geocode_calls"`
	CacheHits        int       `json:"cache_hits"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// PluginStats is the response for GET /api/v1/stats.
type PluginStats struct {
	TotalCacheEntries   int     `json:"total_cache_entries"`
	TotalGeofences      int     `json:"total_geofences"`
	ActiveGeofences     int     `json:"active_geofences"`
	TotalGeofenceEvents int     `json:"total_geofence_events"`
	TotalPlaces         int     `json:"total_places"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
}

// GeofenceEvalResult is one entry in the check-geofence response.
type GeofenceEvalResult struct {
	GeofenceID    string  `json:"geofence_id"`
	GeofenceName  string  `json:"geofence_name"`
	Inside        bool    `json:"inside"`
	DistanceMeters float64 `json:"distance_meters"`
}
