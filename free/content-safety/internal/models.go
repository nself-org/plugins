package internal

import "time"

// Evidence represents a trust-safety evidence record.
type Evidence struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	Type            string         `json:"type"`
	Content         map[string]any `json:"content"`
	Reason          string         `json:"reason"`
	Source          string         `json:"source"`
	WorkspaceID     string         `json:"workspace_id"`
	CollectorID     *string        `json:"collector_id"`
	CollectorRole   *string        `json:"collector_role"`
	Priority        string         `json:"priority"`
	IsEncrypted     bool           `json:"is_encrypted"`
	Status          string         `json:"status"`
	LegalHoldID     *string        `json:"legal_hold_id"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// LegalHold represents a legal hold.
type LegalHold struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	Name            string         `json:"name"`
	Description     *string        `json:"description"`
	Scope           map[string]any `json:"scope"`
	Criteria        map[string]any `json:"criteria"`
	Status          string         `json:"status"`
	CreatedBy       *string        `json:"created_by"`
	ReleasedBy      *string        `json:"released_by"`
	ReleasedAt      *time.Time     `json:"released_at"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// EvidenceExport represents an evidence export job.
type EvidenceExport struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	LegalHoldID     *string        `json:"legal_hold_id"`
	EvidenceIDs     []string       `json:"evidence_ids"`
	Format          string         `json:"format"`
	Status          string         `json:"status"`
	RequestedBy     *string        `json:"requested_by"`
	DownloadURL     *string        `json:"download_url"`
	ExpiresAt       *time.Time     `json:"expires_at"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// SpamRule represents a spam detection rule.
type SpamRule struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	Name            string         `json:"name"`
	Description     *string        `json:"description"`
	RuleType        string         `json:"rule_type"`
	Pattern         *string        `json:"pattern"`
	Conditions      map[string]any `json:"conditions"`
	Action          string         `json:"action"`
	IsEnabled       bool           `json:"is_enabled"`
	Priority        int            `json:"priority"`
	CreatedBy       *string        `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// SpamConfig represents per-workspace spam configuration.
type SpamConfig struct {
	WorkspaceID         string         `json:"workspace_id"`
	SourceAccountID     string         `json:"source_account_id"`
	Sensitivity         string         `json:"sensitivity"`
	AutoDelete          bool           `json:"auto_delete"`
	NotifyModerators    bool           `json:"notify_moderators"`
	QuarantineThreshold float64        `json:"quarantine_threshold"`
	Config              map[string]any `json:"config"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// RaidEvent represents a detected raid event.
type RaidEvent struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	WorkspaceID     string         `json:"workspace_id"`
	ChannelID       *string        `json:"channel_id"`
	EventType       string         `json:"event_type"`
	Severity        string         `json:"severity"`
	DetectedAt      time.Time      `json:"detected_at"`
	ResolvedAt      *time.Time     `json:"resolved_at"`
	Details         map[string]any `json:"details"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Lockdown represents an active or past lockdown state.
type Lockdown struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	WorkspaceID     string         `json:"workspace_id"`
	ChannelID       *string        `json:"channel_id"`
	Level           string         `json:"level"`
	Reason          *string        `json:"reason"`
	ActivatedBy     *string        `json:"activated_by"`
	DeactivatedBy   *string        `json:"deactivated_by"`
	DeactivatedAt   *time.Time     `json:"deactivated_at"`
	IsActive        bool           `json:"is_active"`
	Config          map[string]any `json:"config"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// AbuseScore represents a user trust/abuse score.
type AbuseScore struct {
	UserID          string         `json:"user_id"`
	SourceAccountID string         `json:"source_account_id"`
	TrustScore      float64        `json:"trust_score"`
	RiskLevel       string         `json:"risk_level"`
	TotalEvents     int            `json:"total_events"`
	PositiveEvents  int            `json:"positive_events"`
	NegativeEvents  int            `json:"negative_events"`
	LastEventAt     *time.Time     `json:"last_event_at"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
