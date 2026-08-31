package internal

import "time"

// DDNSConfig represents a row in np_ddns_config.
type DDNSConfig struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Provider        string     `json:"provider"`
	Domain          string     `json:"domain"`
	Hostname        string     `json:"hostname"`
	Token           string     `json:"token"`
	APIKey          *string    `json:"api_key"`
	ZoneID          *string    `json:"zone_id"`
	RecordID        *string    `json:"record_id"`
	RecordType      string     `json:"record_type"`
	TTL             int        `json:"ttl"`
	CurrentIP       *string    `json:"current_ip"`
	LastUpdated     *time.Time `json:"last_updated"`
	CheckInterval   int        `json:"check_interval"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UpdateLog represents a row in np_ddns_update_log.
type UpdateLog struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ConfigID        string    `json:"config_id"`
	Provider        string    `json:"provider"`
	Domain          string    `json:"domain"`
	OldIP           *string   `json:"old_ip"`
	NewIP           string    `json:"new_ip"`
	Status          string    `json:"status"`
	ResponseCode    *int      `json:"response_code"`
	ResponseMessage *string   `json:"response_message"`
	Error           *string   `json:"error"`
	DurationMS      int       `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

// UpdateResult summarises a single DNS update attempt.
type UpdateResult struct {
	ConfigID string `json:"config_id"`
	Provider string `json:"provider"`
	Domain   string `json:"domain"`
	OldIP    string `json:"old_ip"`
	NewIP    string `json:"new_ip"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}
