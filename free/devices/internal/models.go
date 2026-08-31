package internal

import "time"

// Device represents a row in np_dev_devices.
type Device struct {
	ID                  string         `json:"id"`
	SourceAccountID     string         `json:"source_account_id"`
	AppID               string         `json:"app_id"`
	DeviceID            string         `json:"device_id"`
	Name                *string        `json:"name"`
	DeviceType          string         `json:"device_type"`
	Model               *string        `json:"model"`
	FirmwareVersion     *string        `json:"firmware_version"`
	Status              string         `json:"status"`
	TrustLevel          string         `json:"trust_level"`
	EnrollmentToken     *string        `json:"enrollment_token,omitempty"`
	EnrollmentChallenge *string        `json:"enrollment_challenge,omitempty"`
	EnrolledAt          *time.Time     `json:"enrolled_at"`
	EnrolledBy          *string        `json:"enrolled_by"`
	PublicKey           *string        `json:"public_key,omitempty"`
	LastSeenAt          *time.Time     `json:"last_seen_at"`
	LastIP              *string        `json:"last_ip"`
	Capabilities        JSONField      `json:"capabilities"`
	Config              JSONField      `json:"config"`
	Labels              JSONField      `json:"labels"`
	Metadata            JSONField      `json:"metadata"`
	RevokedAt           *time.Time     `json:"revoked_at"`
	RevokedBy           *string        `json:"revoked_by"`
	RevokeReason        *string        `json:"revoke_reason"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Command represents a row in np_dev_commands.
type Command struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	AppID            string     `json:"app_id"`
	DeviceID         string     `json:"device_id"`
	CommandType      string     `json:"command_type"`
	CommandIDField   string     `json:"command_id"`
	Payload          JSONField  `json:"payload"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DispatchedAt     time.Time  `json:"dispatched_at"`
	AckedAt          *time.Time `json:"acked_at"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	Result           JSONField  `json:"result"`
	Error            *string    `json:"error"`
	TimeoutSeconds   int        `json:"timeout_seconds"`
	Deadline         *time.Time `json:"deadline"`
	RetryCount       int        `json:"retry_count"`
	MaxRetries       int        `json:"max_retries"`
	IdempotencyKey   string     `json:"idempotency_key"`
	Metadata         JSONField  `json:"metadata"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Telemetry represents a row in np_dev_telemetry.
type Telemetry struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	AppID            string    `json:"app_id"`
	DeviceID         string    `json:"device_id"`
	TelemetryType    string    `json:"telemetry_type"`
	Data             JSONField `json:"data"`
	RecordedAt       time.Time `json:"recorded_at"`
	ReceivedAt       time.Time `json:"received_at"`
}

// IngestSession represents a row in np_dev_ingest_sessions.
type IngestSession struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	AppID            string     `json:"app_id"`
	DeviceID         string     `json:"device_id"`
	StreamID         string     `json:"stream_id"`
	Status           string     `json:"status"`
	IngestURL        *string    `json:"ingest_url"`
	Protocol         string     `json:"protocol"`
	Channel          *string    `json:"channel"`
	Quality          *string    `json:"quality"`
	BitrateKbps      *int       `json:"bitrate_kbps"`
	FPS              *float64   `json:"fps"`
	Resolution       *string    `json:"resolution"`
	StartedAt        *time.Time `json:"started_at"`
	LastHeartbeatAt  *time.Time `json:"last_heartbeat_at"`
	EndedAt          *time.Time `json:"ended_at"`
	BytesIngested    int64      `json:"bytes_ingested"`
	FramesDropped    int64      `json:"frames_dropped"`
	ErrorCount       int        `json:"error_count"`
	LastError        *string    `json:"last_error"`
	Metadata         JSONField  `json:"metadata"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// AuditLog represents a row in np_dev_audit_log.
type AuditLog struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	AppID            string    `json:"app_id"`
	DeviceID         *string   `json:"device_id"`
	Action           string    `json:"action"`
	ActorID          *string   `json:"actor_id"`
	Details          JSONField `json:"details"`
	CreatedAt        time.Time `json:"created_at"`
}

// BootstrapToken represents a row in np_dev_bootstrap_tokens.
type BootstrapToken struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	Name             string     `json:"name"`
	Token            string     `json:"token"`
	Capabilities     JSONField  `json:"capabilities"`
	ExpiresAt        time.Time  `json:"expires_at"`
	Used             bool       `json:"used"`
	UsedByDeviceID   *string    `json:"used_by_device_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

// FleetStats is returned by GET /api/v1/stats.
type FleetStats struct {
	TotalDevices         int        `json:"total_devices"`
	EnrolledDevices      int        `json:"enrolled_devices"`
	OnlineDevices        int        `json:"online_devices"`
	SuspendedDevices     int        `json:"suspended_devices"`
	RevokedDevices       int        `json:"revoked_devices"`
	TotalCommands        int        `json:"total_commands"`
	PendingCommands      int        `json:"pending_commands"`
	SucceededCommands    int        `json:"succeeded_commands"`
	FailedCommands       int        `json:"failed_commands"`
	ActiveIngestSessions int        `json:"active_ingest_sessions"`
	TotalTelemetryRecords int       `json:"total_telemetry_records"`
	LastActivity         *time.Time `json:"last_activity"`
}

// JSONField is a raw JSON value that serialises as-is.
type JSONField []byte

func (j JSONField) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSONField) UnmarshalJSON(data []byte) error {
	*j = append((*j)[0:0], data...)
	return nil
}
