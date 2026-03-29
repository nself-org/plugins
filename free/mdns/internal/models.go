package internal

import (
	"encoding/json"
	"time"
)

// ServiceRecord represents a row in np_mdns_services.
type ServiceRecord struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	ServiceName     string          `json:"service_name"`
	ServiceType     string          `json:"service_type"`
	Port            int             `json:"port"`
	Host            string          `json:"host"`
	Domain          string          `json:"domain"`
	TxtRecords      json.RawMessage `json:"txt_records"`
	IsAdvertised    bool            `json:"is_advertised"`
	IsActive        bool            `json:"is_active"`
	LastSeenAt      time.Time       `json:"last_seen_at"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// DiscoveryLogRecord represents a row in np_mdns_discovery_log.
type DiscoveryLogRecord struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	ServiceType     string          `json:"service_type"`
	ServiceName     string          `json:"service_name"`
	Host            string          `json:"host"`
	Port            int             `json:"port"`
	Addresses       []string        `json:"addresses"`
	TxtRecords      json.RawMessage `json:"txt_records"`
	DiscoveredAt    time.Time       `json:"discovered_at"`
	LastSeenAt      time.Time       `json:"last_seen_at"`
	IsAvailable     bool            `json:"is_available"`
	Metadata        json.RawMessage `json:"metadata"`
}

// MdnsStats holds aggregate statistics for the plugin.
type MdnsStats struct {
	TotalServices      int `json:"total_services"`
	ActiveServices     int `json:"active_services"`
	AdvertisedServices int `json:"advertised_services"`
	TotalDiscovered    int `json:"total_discovered"`
	AvailableDiscovered int `json:"available_discovered"`
}

// --- Request types ---

// CreateServiceRequest is the JSON body for POST /api/services.
type CreateServiceRequest struct {
	ServiceName string           `json:"service_name"`
	ServiceType string           `json:"service_type,omitempty"`
	Port        int              `json:"port"`
	Host        string           `json:"host,omitempty"`
	Domain      string           `json:"domain,omitempty"`
	TxtRecords  *json.RawMessage `json:"txt_records,omitempty"`
	Metadata    *json.RawMessage `json:"metadata,omitempty"`
}

// UpdateServiceRequest is the JSON body for PUT /api/services/{id}.
type UpdateServiceRequest struct {
	ServiceName *string          `json:"service_name,omitempty"`
	ServiceType *string          `json:"service_type,omitempty"`
	Port        *int             `json:"port,omitempty"`
	Host        *string          `json:"host,omitempty"`
	Domain      *string          `json:"domain,omitempty"`
	TxtRecords  *json.RawMessage `json:"txt_records,omitempty"`
	IsActive    *bool            `json:"is_active,omitempty"`
	Metadata    *json.RawMessage `json:"metadata,omitempty"`
}

// DiscoverRequest is the JSON body for POST /api/discover.
type DiscoverRequest struct {
	ServiceType string           `json:"service_type,omitempty"`
	Services    []DiscoverEntry  `json:"services,omitempty"`
}

// DiscoverEntry represents a single discovered service to upsert.
type DiscoverEntry struct {
	ServiceType string           `json:"service_type"`
	ServiceName string           `json:"service_name"`
	Host        string           `json:"host"`
	Port        int              `json:"port"`
	Addresses   []string         `json:"addresses,omitempty"`
	TxtRecords  *json.RawMessage `json:"txt_records,omitempty"`
	Metadata    *json.RawMessage `json:"metadata,omitempty"`
}

// --- Response types ---

// ListResponse wraps a paginated list of items.
type ListResponse struct {
	Items  interface{} `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
