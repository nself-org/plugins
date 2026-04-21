package internal

import "time"

// AuditEvent represents a single security-relevant event in the audit log.
// The table is append-only: DELETE and UPDATE are blocked at the database level
// via Postgres row-level security policies.
type AuditEvent struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	ActorUserID     string         `json:"actor_user_id"`
	ActorType       string         `json:"actor_type"`       // user | system | plugin
	EventType       string         `json:"event_type"`       // auth.login | auth.logout | auth.login_failed | auth.mfa_enabled | privilege.change | secret.accessed | plugin.installed | plugin.uninstalled
	ResourceType    string         `json:"resource_type"`
	ResourceID      string         `json:"resource_id"`
	IPAddress       string         `json:"ip_address"`
	UserAgent       string         `json:"user_agent"`
	Metadata        map[string]any `json:"metadata"`
	Severity        string         `json:"severity"` // info | warning | critical
	// Inter-plugin tracing columns (S43-T18). Both are empty for non-plugin events.
	SourcePlugin string    `json:"source_plugin"` // plugin that made the call (X-Source-Plugin)
	TargetPlugin string    `json:"target_plugin"` // plugin that received the call
	CreatedAt    time.Time `json:"created_at"`
}

// IngestRequest is the JSON body accepted by POST /events.
type IngestRequest struct {
	SourceAccountID string         `json:"source_account_id"`
	ActorUserID     string         `json:"actor_user_id"`
	ActorType       string         `json:"actor_type"`
	EventType       string         `json:"event_type"`
	ResourceType    string         `json:"resource_type"`
	ResourceID      string         `json:"resource_id"`
	IPAddress       string         `json:"ip_address"`
	UserAgent       string         `json:"user_agent"`
	Metadata        map[string]any `json:"metadata"`
	Severity        string         `json:"severity"`
	// Inter-plugin call attribution. SourcePlugin is auto-populated from
	// X-Source-Plugin header when not explicitly set. S43-T18.
	SourcePlugin string `json:"source_plugin,omitempty"`
	TargetPlugin string `json:"target_plugin,omitempty"`
}

// ListResponse is the JSON envelope returned by GET /events.
type ListResponse struct {
	Events []*AuditEvent `json:"events"`
	Total  int64         `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// validActorTypes is the set of accepted actor_type values.
var validActorTypes = map[string]bool{
	"user":   true,
	"system": true,
	"plugin": true,
}

// validEventTypes is the set of accepted event_type values.
var validEventTypes = map[string]bool{
	"auth.login":           true,
	"auth.logout":          true,
	"auth.login_failed":    true,
	"auth.mfa_enabled":     true,
	"privilege.change":     true,
	"secret.accessed":      true,
	"plugin.installed":     true,
	"plugin.uninstalled":   true,
}

// validSeverities is the set of accepted severity values.
var validSeverities = map[string]bool{
	"info":     true,
	"warning":  true,
	"critical": true,
}
