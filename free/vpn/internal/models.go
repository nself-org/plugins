package internal

import "time"

// ---------------------------------------------------------------------------
// Database record types
// ---------------------------------------------------------------------------

// Provider represents a row in np_vpn_providers.
type Provider struct {
	ID                       string                 `json:"id"`
	Name                     string                 `json:"name"`
	DisplayName              string                 `json:"display_name"`
	CLIAvailable             bool                   `json:"cli_available"`
	CLICommand               *string                `json:"cli_command,omitempty"`
	APIAvailable             bool                   `json:"api_available"`
	APIEndpoint              *string                `json:"api_endpoint,omitempty"`
	PortForwardingSupported  bool                   `json:"port_forwarding_supported"`
	P2PAllServers            bool                   `json:"p2p_all_servers"`
	P2PServerCount           int                    `json:"p2p_server_count"`
	TotalServers             int                    `json:"total_servers"`
	TotalCountries           int                    `json:"total_countries"`
	WireguardSupported       bool                   `json:"wireguard_supported"`
	OpenVPNSupported         bool                   `json:"openvpn_supported"`
	KillSwitchAvailable      bool                   `json:"kill_switch_available"`
	SplitTunnelingAvailable  bool                   `json:"split_tunneling_available"`
	Config                   map[string]interface{} `json:"config"`
	SourceAccountID          string                 `json:"source_account_id"`
	CreatedAt                time.Time              `json:"created_at"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

// Credential represents a row in np_vpn_credentials.
type Credential struct {
	ID                  string                 `json:"id"`
	ProviderID          string                 `json:"provider_id"`
	Username            *string                `json:"username,omitempty"`
	PasswordEncrypted   *string                `json:"password_encrypted,omitempty"`
	APIKeyEncrypted     *string                `json:"api_key_encrypted,omitempty"`
	APITokenEncrypted   *string                `json:"api_token_encrypted,omitempty"`
	AccountNumber       *string                `json:"account_number,omitempty"`
	PrivateKeyEncrypted *string                `json:"private_key_encrypted,omitempty"`
	AdditionalData      map[string]interface{} `json:"additional_data,omitempty"`
	ExpiresAt           *time.Time             `json:"expires_at,omitempty"`
	SourceAccountID     string                 `json:"source_account_id"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// Server represents a row in np_vpn_servers.
type Server struct {
	ID                      string                 `json:"id"`
	ProviderID              string                 `json:"provider_id"`
	Hostname                string                 `json:"hostname"`
	IPAddress               string                 `json:"ip_address"`
	IPv6Address             *string                `json:"ipv6_address,omitempty"`
	CountryCode             string                 `json:"country_code"`
	CountryName             string                 `json:"country_name"`
	City                    *string                `json:"city,omitempty"`
	Region                  *string                `json:"region,omitempty"`
	Latitude                *float64               `json:"latitude,omitempty"`
	Longitude               *float64               `json:"longitude,omitempty"`
	P2PSupported            bool                   `json:"p2p_supported"`
	PortForwardingSupported bool                   `json:"port_forwarding_supported"`
	Protocols               []string               `json:"protocols"`
	Load                    *int                   `json:"load,omitempty"`
	Capacity                *int                   `json:"capacity,omitempty"`
	Status                  string                 `json:"status"`
	Features                []string               `json:"features"`
	PublicKey               *string                `json:"public_key,omitempty"`
	EndpointPort            *int                   `json:"endpoint_port,omitempty"`
	Owned                   bool                   `json:"owned"`
	Metadata                map[string]interface{} `json:"metadata"`
	SourceAccountID         string                 `json:"source_account_id"`
	LastSeen                time.Time              `json:"last_seen"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
}

// Connection represents a row in np_vpn_connections.
type Connection struct {
	ID                string                 `json:"id"`
	ProviderID        string                 `json:"provider_id"`
	ServerID          *string                `json:"server_id,omitempty"`
	Protocol          string                 `json:"protocol"`
	Status            string                 `json:"status"`
	LocalIP           *string                `json:"local_ip,omitempty"`
	VPNIP             *string                `json:"vpn_ip,omitempty"`
	InterfaceName     *string                `json:"interface_name,omitempty"`
	DNSServers        []string               `json:"dns_servers"`
	ConnectedAt       *time.Time             `json:"connected_at,omitempty"`
	DisconnectedAt    *time.Time             `json:"disconnected_at,omitempty"`
	DurationSeconds   *int                   `json:"duration_seconds,omitempty"`
	BytesSent         int64                  `json:"bytes_sent"`
	BytesReceived     int64                  `json:"bytes_received"`
	ErrorMessage      *string                `json:"error_message,omitempty"`
	KillSwitchEnabled bool                   `json:"kill_switch_enabled"`
	PortForwarded     *int                   `json:"port_forwarded,omitempty"`
	RequestedBy       *string                `json:"requested_by,omitempty"`
	Metadata          map[string]interface{} `json:"metadata"`
	SourceAccountID   string                 `json:"source_account_id"`
	CreatedAt         time.Time              `json:"created_at"`
}

// Download represents a row in np_vpn_downloads.
type Download struct {
	ID              string                 `json:"id"`
	ConnectionID    *string                `json:"connection_id,omitempty"`
	MagnetLink      string                 `json:"magnet_link"`
	InfoHash        string                 `json:"info_hash"`
	Name            *string                `json:"name,omitempty"`
	DestinationPath string                 `json:"destination_path"`
	Status          string                 `json:"status"`
	Progress        float64                `json:"progress"`
	BytesDownloaded int64                  `json:"bytes_downloaded"`
	BytesTotal      *int64                 `json:"bytes_total,omitempty"`
	DownloadSpeed   int64                  `json:"download_speed"`
	UploadSpeed     int64                  `json:"upload_speed"`
	Peers           int                    `json:"peers"`
	Seeds           int                    `json:"seeds"`
	ETASeconds      *int                   `json:"eta_seconds,omitempty"`
	RequestedBy     string                 `json:"requested_by"`
	ProviderID      string                 `json:"provider_id"`
	ServerID        *string                `json:"server_id,omitempty"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	ErrorMessage    *string                `json:"error_message,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
	SourceAccountID string                 `json:"source_account_id"`
	CreatedAt       time.Time              `json:"created_at"`
}

// ConnectionLog represents a row in np_vpn_connection_logs.
type ConnectionLog struct {
	ID              string                 `json:"id"`
	ConnectionID    string                 `json:"connection_id"`
	Timestamp       time.Time              `json:"timestamp"`
	EventType       string                 `json:"event_type"`
	Message         string                 `json:"message"`
	Details         map[string]interface{} `json:"details"`
	SourceAccountID string                 `json:"source_account_id"`
}

// ServerPerformance represents a row in np_vpn_server_performance.
type ServerPerformance struct {
	ID                 string    `json:"id"`
	ServerID           string    `json:"server_id"`
	Timestamp          time.Time `json:"timestamp"`
	PingMs             *int      `json:"ping_ms,omitempty"`
	DownloadSpeedMbps  *float64  `json:"download_speed_mbps,omitempty"`
	UploadSpeedMbps    *float64  `json:"upload_speed_mbps,omitempty"`
	LoadPercentage     *int      `json:"load_percentage,omitempty"`
	SuccessRate        *float64  `json:"success_rate,omitempty"`
	AvgConnectionTimeMs *int     `json:"avg_connection_time_ms,omitempty"`
	SourceAccountID    string    `json:"source_account_id"`
}

// LeakTest represents a row in np_vpn_leak_tests.
type LeakTest struct {
	ID              string                 `json:"id"`
	ConnectionID    string                 `json:"connection_id"`
	TestType        string                 `json:"test_type"`
	Passed          bool                   `json:"passed"`
	ExpectedValue   *string                `json:"expected_value,omitempty"`
	ActualValue     *string                `json:"actual_value,omitempty"`
	Details         map[string]interface{} `json:"details"`
	TestedAt        time.Time              `json:"tested_at"`
	SourceAccountID string                 `json:"source_account_id"`
}

// ---------------------------------------------------------------------------
// API request / response types
// ---------------------------------------------------------------------------

// ConnectRequest is the JSON body for POST /api/connect.
type ConnectRequest struct {
	Provider       string  `json:"provider"`
	Region         *string `json:"region,omitempty"`
	City           *string `json:"city,omitempty"`
	Server         *string `json:"server,omitempty"`
	Protocol       *string `json:"protocol,omitempty"`
	KillSwitch     *bool   `json:"kill_switch,omitempty"`
	PortForwarding *bool   `json:"port_forwarding,omitempty"`
	RequestedBy    *string `json:"requested_by,omitempty"`
}

// ConnectResponse is the response for POST /api/connect.
type ConnectResponse struct {
	ConnectionID string     `json:"connection_id"`
	Provider     string     `json:"provider"`
	Server       string     `json:"server"`
	VPNIP        string     `json:"vpn_ip"`
	Interface    string     `json:"interface"`
	DNSServers   []string   `json:"dns_servers"`
	PortForwarded *int      `json:"port_forwarded,omitempty"`
	ConnectedAt  *time.Time `json:"connected_at"`
}

// DownloadRequest is the JSON body for POST /api/download.
type DownloadRequest struct {
	MagnetLink  string  `json:"magnet_link"`
	Destination *string `json:"destination,omitempty"`
	Provider    *string `json:"provider,omitempty"`
	Region      *string `json:"region,omitempty"`
	RequestedBy string  `json:"requested_by"`
}

// DownloadResponse is the response for POST /api/download.
type DownloadResponse struct {
	DownloadID string     `json:"download_id"`
	Name       *string    `json:"name,omitempty"`
	Status     string     `json:"status"`
	Provider   string     `json:"provider"`
	Server     *string    `json:"server,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CredentialRequest is the JSON body for POST /api/providers/{id}/credentials.
type CredentialRequest struct {
	Username      *string `json:"username,omitempty"`
	Password      *string `json:"password,omitempty"`
	Token         *string `json:"token,omitempty"`
	AccountNumber *string `json:"account_number,omitempty"`
	APIKey        *string `json:"api_key,omitempty"`
}

// ServerSyncRequest is the JSON body for POST /api/servers/sync.
type ServerSyncRequest struct {
	Provider string `json:"provider"`
}

// HealthResponse is the response for GET /api/health.
type HealthResponse struct {
	VPNConnected bool    `json:"vpn_connected"`
	DNSLeak      bool    `json:"dns_leak"`
	WebRTCLeak   bool    `json:"webrtc_leak"`
	IPv6Leak     bool    `json:"ipv6_leak"`
	LastTest     *string `json:"last_test"`
}

// StatusResponse is the response for GET /api/status.
type StatusResponse struct {
	Connected        bool    `json:"connected"`
	ConnectionID     *string `json:"connection_id,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Server           *string `json:"server,omitempty"`
	VPNIP            *string `json:"vpn_ip,omitempty"`
	Interface        *string `json:"interface,omitempty"`
	Protocol         *string `json:"protocol,omitempty"`
	UptimeSeconds    *int    `json:"uptime_seconds,omitempty"`
	BytesSent        *int64  `json:"bytes_sent,omitempty"`
	BytesReceived    *int64  `json:"bytes_received,omitempty"`
	PortForwarded    *int    `json:"port_forwarded,omitempty"`
	KillSwitchEnabled *bool  `json:"kill_switch_enabled,omitempty"`
}

// Statistics is the response for GET /api/stats.
type Statistics struct {
	TotalConnections    int              `json:"total_connections"`
	ActiveConnections   int              `json:"active_connections"`
	TotalDownloads      int              `json:"total_downloads"`
	ActiveDownloads     int              `json:"active_downloads"`
	TotalBytesDownloaded string          `json:"total_bytes_downloaded"`
	Providers           []ProviderStat   `json:"providers"`
	TopServers          []ServerStat     `json:"top_servers"`
}

// ProviderStat is a sub-object in Statistics.
type ProviderStat struct {
	Provider         string  `json:"provider"`
	Connections      int     `json:"connections"`
	UptimePercentage float64 `json:"uptime_percentage"`
	AvgSpeedMbps     float64 `json:"avg_speed_mbps"`
}

// ServerStat is a sub-object in Statistics.
type ServerStat struct {
	Server       string  `json:"server"`
	Provider     string  `json:"provider"`
	Country      string  `json:"country"`
	Connections  int     `json:"connections"`
	AvgSpeedMbps float64 `json:"avg_speed_mbps"`
}

// ServerFilter holds query params for GET /api/servers.
type ServerFilter struct {
	Provider       string
	Country        string
	P2POnly        bool
	PortForwarding bool
	Limit          int
}
