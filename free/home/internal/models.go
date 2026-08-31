package internal

import (
	"encoding/json"
	"time"
)

// Connection represents a Home Assistant connection configuration.
type Connection struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Name            string    `json:"name"`
	HAUrl           string    `json:"ha_url"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

// Device represents a cached Home Assistant entity/device.
type Device struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	EntityID        string          `json:"entity_id"`
	FriendlyName    *string         `json:"friendly_name,omitempty"`
	Domain          string          `json:"domain"`
	State           *string         `json:"state,omitempty"`
	Attributes      json.RawMessage `json:"attributes,omitempty"`
	LastUpdated     time.Time       `json:"last_updated"`
}

// CommandLog records a service call made to Home Assistant.
type CommandLog struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	EntityID        string          `json:"entity_id"`
	Service         string          `json:"service"`
	Data            json.RawMessage `json:"data,omitempty"`
	Result          *string         `json:"result,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// CommandRequest is the payload for POST /home/command.
type CommandRequest struct {
	EntityID string          `json:"entity_id"`
	Service  string          `json:"service"`
	Data     json.RawMessage `json:"data,omitempty"`
}
